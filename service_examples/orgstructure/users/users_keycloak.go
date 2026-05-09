package users

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"encore.dev/rlog"

	"encore.app/auth/authhandler"
)

var secrets struct {
	KeycloakIssuerURL     string
	KeycloakAdminUser     string
	KeycloakAdminPassword string
}

var kcAdmin = &keycloakAdminClient{
	httpClient: &http.Client{Timeout: 10 * time.Second},
}

type keycloakAdminClient struct {
	httpClient  *http.Client
	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

func (k *keycloakAdminClient) baseURL() string {
	u, err := url.Parse(secrets.KeycloakIssuerURL)
	if err != nil || secrets.KeycloakIssuerURL == "" {
		return ""
	}
	parts := strings.SplitN(u.Path, "/realms/", 2)
	u.Path = parts[0]
	return u.String()
}

func (k *keycloakAdminClient) realm() string {
	parts := strings.SplitN(secrets.KeycloakIssuerURL, "/realms/", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSuffix(parts[1], "/")
}

// adminToken returns a valid master-realm admin token, refreshing 30 s before expiry.
func (k *keycloakAdminClient) adminToken() (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.cachedToken != "" && time.Now().Before(k.tokenExpiry) {
		return k.cachedToken, nil
	}

	tokenURL := fmt.Sprintf("%s/realms/master/protocol/openid-connect/token", k.baseURL())
	form := url.Values{
		"grant_type": {"password"},
		"client_id":  {"admin-cli"},
		"username":   {secrets.KeycloakAdminUser},
		"password":   {secrets.KeycloakAdminPassword},
	}

	resp, err := k.httpClient.Post(tokenURL, "application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("keycloak admin token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("keycloak admin token: status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("keycloak admin token decode: %w", err)
	}

	k.cachedToken = result.AccessToken
	k.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-30) * time.Second)
	return k.cachedToken, nil
}

type kcRoleRepresentation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (k *keycloakAdminClient) getRoleByName(token, roleName string) (*kcRoleRepresentation, error) {
	roleURL := fmt.Sprintf("%s/admin/realms/%s/roles/%s", k.baseURL(), k.realm(), roleName)
	req, _ := http.NewRequest(http.MethodGet, roleURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get role %q: status %d: %s", roleName, resp.StatusCode, body)
	}

	var role kcRoleRepresentation
	if err := json.NewDecoder(resp.Body).Decode(&role); err != nil {
		return nil, err
	}
	return &role, nil
}

func (k *keycloakAdminClient) getCurrentRealmRoles(token, kcUserID string) ([]kcRoleRepresentation, error) {
	rolesURL := fmt.Sprintf("%s/admin/realms/%s/users/%s/role-mappings/realm",
		k.baseURL(), k.realm(), kcUserID)
	req, _ := http.NewRequest(http.MethodGet, rolesURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get realm roles: status %d: %s", resp.StatusCode, body)
	}

	var roles []kcRoleRepresentation
	if err := json.NewDecoder(resp.Body).Decode(&roles); err != nil {
		return nil, err
	}
	return roles, nil
}

func (k *keycloakAdminClient) addRealmRole(token, kcUserID string, role *kcRoleRepresentation) error {
	rolesURL := fmt.Sprintf("%s/admin/realms/%s/users/%s/role-mappings/realm",
		k.baseURL(), k.realm(), kcUserID)
	body, _ := json.Marshal([]kcRoleRepresentation{*role})
	req, _ := http.NewRequest(http.MethodPost, rolesURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add realm role %q: status %d: %s", role.Name, resp.StatusCode, respBody)
	}
	return nil
}

func (k *keycloakAdminClient) removeRealmRoles(token, kcUserID string, roles []kcRoleRepresentation) error {
	if len(roles) == 0 {
		return nil
	}
	rolesURL := fmt.Sprintf("%s/admin/realms/%s/users/%s/role-mappings/realm",
		k.baseURL(), k.realm(), kcUserID)
	body, _ := json.Marshal(roles)
	req, _ := http.NewRequest(http.MethodDelete, rolesURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remove realm roles: status %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// businessRoles is the set of realm role names owned by the backend.
var businessRoles = map[string]bool{
	string(authhandler.RoleSA):  true,
	string(authhandler.RoleADM): true,
	string(authhandler.RoleHR):  true,
	string(authhandler.RoleEMP): true,
}

// syncRoleToKeycloak replaces the user's business roles with newRole.
// Errors are logged but do not fail the API response — PostgreSQL is the source of truth.
func syncRoleToKeycloak(ctx context.Context, kcUserID string, newRole authhandler.UserRole) {
	if secrets.KeycloakIssuerURL == "" || secrets.KeycloakAdminUser == "" {
		rlog.Warn("keycloak sync skipped: admin credentials not configured")
		return
	}

	token, err := kcAdmin.adminToken()
	if err != nil {
		rlog.Error("keycloak sync: failed to get admin token", "err", err.Error())
		return
	}

	current, err := kcAdmin.getCurrentRealmRoles(token, kcUserID)
	if err != nil {
		rlog.Error("keycloak sync: failed to get current roles", "kcUserID", kcUserID, "err", err.Error())
		return
	}
	var toRemove []kcRoleRepresentation
	for _, r := range current {
		if businessRoles[r.Name] {
			toRemove = append(toRemove, r)
		}
	}
	if err := kcAdmin.removeRealmRoles(token, kcUserID, toRemove); err != nil {
		rlog.Error("keycloak sync: failed to remove old roles", "kcUserID", kcUserID, "err", err.Error())
		return
	}

	role, err := kcAdmin.getRoleByName(token, string(newRole))
	if err != nil {
		rlog.Error("keycloak sync: failed to resolve role", "role", newRole, "err", err.Error())
		return
	}
	if err := kcAdmin.addRealmRole(token, kcUserID, role); err != nil {
		rlog.Error("keycloak sync: failed to assign role", "kcUserID", kcUserID, "role", newRole, "err", err.Error())
		return
	}
	rlog.Info("keycloak sync: role updated", "kcUserID", kcUserID, "newRole", string(newRole))
}

// ════ USER CREATION & ATTRIBUTE SYNC ════

// kcUserRepresentation is the Keycloak UserRepresentation used for create/update.
type kcUserRepresentation struct {
	ID          string              `json:"id,omitempty"`
	Username    string              `json:"username,omitempty"`
	Email       string              `json:"email,omitempty"`
	FirstName   string              `json:"firstName,omitempty"`
	LastName    string              `json:"lastName,omitempty"`
	Enabled     *bool               `json:"enabled,omitempty"`
	Credentials []kcCredentialRep   `json:"credentials,omitempty"`
	Attributes  map[string][]string `json:"attributes,omitempty"`
}

type kcCredentialRep struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary"`
}

// createUser creates a Keycloak user and returns their UUID.
// The ID is extracted from the Location response header.
func (k *keycloakAdminClient) createUser(token string, rep kcUserRepresentation) (string, error) {
	usersURL := fmt.Sprintf("%s/admin/realms/%s/users", k.baseURL(), k.realm())
	body, _ := json.Marshal(rep)

	req, _ := http.NewRequest(http.MethodPost, usersURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return "", fmt.Errorf("user with this email already exists in Keycloak")
	}
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create user: status %d: %s", resp.StatusCode, respBody)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("create user: no Location header in response")
	}
	parts := strings.Split(location, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("create user: invalid Location header: %s", location)
	}
	return parts[len(parts)-1], nil
}

// getUser fetches the full UserRepresentation from Keycloak.
func (k *keycloakAdminClient) getUser(token, kcUserID string) (*kcUserRepresentation, error) {
	userURL := fmt.Sprintf("%s/admin/realms/%s/users/%s", k.baseURL(), k.realm(), kcUserID)
	req, _ := http.NewRequest(http.MethodGet, userURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get user: status %d: %s", resp.StatusCode, respBody)
	}

	var rep kcUserRepresentation
	if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
		return nil, err
	}
	return &rep, nil
}

// updateUser sends a full UserRepresentation update to Keycloak.
func (k *keycloakAdminClient) updateUser(token, kcUserID string, rep kcUserRepresentation) error {
	userURL := fmt.Sprintf("%s/admin/realms/%s/users/%s", k.baseURL(), k.realm(), kcUserID)
	body, _ := json.Marshal(rep)

	req, _ := http.NewRequest(http.MethodPut, userURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update user: status %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// deleteUser deletes a Keycloak user — used for rollback when DB insert fails.
func (k *keycloakAdminClient) deleteUser(token, kcUserID string) error {
	userURL := fmt.Sprintf("%s/admin/realms/%s/users/%s", k.baseURL(), k.realm(), kcUserID)
	req, _ := http.NewRequest(http.MethodDelete, userURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete user: status %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// createAndConfigureKeycloakAdmin creates a Keycloak user, assigns the ADM
// realm role, and sets companyId/dzoId attributes so they appear in the JWT.
// Returns the new Keycloak user UUID.
func createAndConfigureKeycloakAdmin(ctx context.Context, email, firstName, lastName, tempPassword string, companyID, dzoID *string) (string, error) {
	if secrets.KeycloakIssuerURL == "" || secrets.KeycloakAdminUser == "" {
		return "", fmt.Errorf("keycloak admin credentials not configured")
	}

	token, err := kcAdmin.adminToken()
	if err != nil {
		return "", fmt.Errorf("keycloak admin token: %w", err)
	}

	enabled := true
	attrs := make(map[string][]string)
	if companyID != nil && *companyID != "" {
		attrs["companyId"] = []string{*companyID}
	}
	if dzoID != nil && *dzoID != "" {
		attrs["dzoId"] = []string{*dzoID}
	}

	rep := kcUserRepresentation{
		Username:  email,
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
		Enabled:   &enabled,
		Credentials: []kcCredentialRep{
			{Type: "password", Value: tempPassword, Temporary: true},
		},
		Attributes: attrs,
	}

	kcUserID, err := kcAdmin.createUser(token, rep)
	if err != nil {
		return "", fmt.Errorf("create keycloak user: %w", err)
	}

	// Assign ADM realm role.
	role, err := kcAdmin.getRoleByName(token, string(authhandler.RoleADM))
	if err != nil {
		_ = kcAdmin.deleteUser(token, kcUserID)
		return "", fmt.Errorf("resolve ADM role: %w", err)
	}
	if err := kcAdmin.addRealmRole(token, kcUserID, role); err != nil {
		_ = kcAdmin.deleteUser(token, kcUserID)
		return "", fmt.Errorf("assign ADM role: %w", err)
	}

	rlog.Info("keycloak: admin created", "kcUserID", kcUserID, "email", email)
	return kcUserID, nil
}

// syncAttributesToKeycloak updates the companyId and dzoId custom attributes
// on a Keycloak user so they appear in subsequent JWT tokens.
// Errors are logged but do not fail the API response.
func syncAttributesToKeycloak(ctx context.Context, kcUserID, companyID, dzoID string) {
	if secrets.KeycloakIssuerURL == "" || secrets.KeycloakAdminUser == "" {
		rlog.Warn("keycloak attr sync skipped: admin credentials not configured")
		return
	}

	token, err := kcAdmin.adminToken()
	if err != nil {
		rlog.Error("keycloak attr sync: failed to get admin token", "err", err.Error())
		return
	}

	// Fetch the full representation to avoid accidentally clearing other fields.
	rep, err := kcAdmin.getUser(token, kcUserID)
	if err != nil {
		rlog.Error("keycloak attr sync: failed to get user", "kcUserID", kcUserID, "err", err.Error())
		return
	}

	if rep.Attributes == nil {
		rep.Attributes = make(map[string][]string)
	}
	if companyID != "" {
		rep.Attributes["companyId"] = []string{companyID}
	}
	if dzoID != "" {
		rep.Attributes["dzoId"] = []string{dzoID}
	}

	if err := kcAdmin.updateUser(token, kcUserID, *rep); err != nil {
		rlog.Error("keycloak attr sync: failed to update user", "kcUserID", kcUserID, "err", err.Error())
		return
	}
	rlog.Info("keycloak attr sync: updated", "kcUserID", kcUserID, "companyID", companyID, "dzoID", dzoID)
}

// generateTempPassword generates a random alphanumeric+symbol password of the
// given length using crypto/rand for security.
func generateTempPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%"
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}

// syncEnabledToKeycloak enables or disables the Keycloak account.
// Disabling prevents new tokens from being issued; existing tokens expire naturally.
// Errors are logged but do not fail the API response.
func syncEnabledToKeycloak(ctx context.Context, kcUserID string, enabled bool) {
	if secrets.KeycloakIssuerURL == "" || secrets.KeycloakAdminUser == "" {
		rlog.Warn("keycloak sync skipped: admin credentials not configured")
		return
	}

	token, err := kcAdmin.adminToken()
	if err != nil {
		rlog.Error("keycloak sync: failed to get admin token", "err", err.Error())
		return
	}

	userURL := fmt.Sprintf("%s/admin/realms/%s/users/%s",
		kcAdmin.baseURL(), kcAdmin.realm(), kcUserID)

	type userUpdate struct {
		Enabled bool `json:"enabled"`
	}
	body, _ := json.Marshal(userUpdate{Enabled: enabled})

	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, userURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := kcAdmin.httpClient.Do(req)
	if err != nil {
		rlog.Error("keycloak sync: failed to update enabled", "kcUserID", kcUserID, "enabled", enabled, "err", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		rlog.Error("keycloak sync: unexpected status on enabled update",
			"status", resp.StatusCode, "body", string(respBody))
		return
	}
	rlog.Info("keycloak sync: user enabled updated", "kcUserID", kcUserID, "enabled", enabled)
}
