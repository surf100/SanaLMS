package employees

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"encore.app/auth/authhandler"
	"encore.dev/rlog"
)

var secrets struct {
	KeycloakIssuerURL         string
	KeycloakAdminClientID     string
	KeycloakAdminClientSecret string
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

func (k *keycloakAdminClient) adminToken() (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.cachedToken != "" && time.Now().Before(k.tokenExpiry) {
		return k.cachedToken, nil
	}

	tokenURL := fmt.Sprintf("%s/protocol/openid-connect/token", secrets.KeycloakIssuerURL)
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {secrets.KeycloakAdminClientID},
		"client_secret": {secrets.KeycloakAdminClientSecret},
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

type kcCreateUserRequest struct {
	Username    string                `json:"username"`
	Email       string                `json:"email"`
	FirstName   string                `json:"firstName"`
	LastName    string                `json:"lastName,omitempty"`
	Enabled     bool                  `json:"enabled"`
	Attributes  map[string][]string   `json:"attributes,omitempty"`
	Credentials []kcCredentialRequest `json:"credentials,omitempty"`
}

type kcCredentialRequest struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary"`
}

// createKeycloakUser создаёт пользователя в keycloak и возвращает его id
func (k *keycloakAdminClient) createKeycloakUser(ctx context.Context, email string, fullName string, companyID string, dzoID string) (string, error) {
	if secrets.KeycloakIssuerURL == "" || secrets.KeycloakAdminClientID == "" {
		rlog.Warn("keycloak not configured, using stub kcUserID")
		return fmt.Sprintf("stub-%s", email), nil
	}

	token, err := kcAdmin.adminToken()
	if err != nil {
		return "", fmt.Errorf("keycloak: get admin token: %w", err)
	}

	usersURL := fmt.Sprintf("%s/admin/realms/%s/users", kcAdmin.baseURL(), kcAdmin.realm())

	firstName, lastName := splitFullName(fullName)
	if firstName == "" {
		firstName = fullName
	}
	if lastName == "" {
		lastName = fullName
	}
	payload := kcCreateUserRequest{
		Username:  email,
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
		Enabled:   true,
		Attributes: map[string][]string{
			"companyId": {companyID},
			"dzoId":     {dzoID},
		},
		Credentials: []kcCredentialRequest{
			{
				Type:      "password",
				Value:     "da",
				Temporary: true,
			},
		},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, usersURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := kcAdmin.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("keycloak: create user request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return k.handleUserConflict(ctx, email)
	}
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("keycloak: create user: status %d: %s", resp.StatusCode, respBody)
	}

	location := resp.Header.Get("Location")
	parts := strings.Split(location, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("keycloak: empty Location header after user creation")
	}
	return parts[len(parts)-1], nil
}

func (k *keycloakAdminClient) assignRealmRoleToUser(ctx context.Context, userID string, role string) error {
	if strings.HasPrefix(userID, "test-kc-") {
		return nil
	}
	token, err := k.adminToken()
	if err != nil {
		return fmt.Errorf("keycloak: get admin token: %w", err)
	}

	baseURL := k.baseURL()
	realm := k.realm()

	// 1. get role by name
	roleURL := fmt.Sprintf("%s/admin/realms/%s/roles/%s", baseURL, realm, url.PathEscape(string(role)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, roleURL, nil)
	if err != nil {
		return fmt.Errorf("keycloak: build get role request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("keycloak: get role request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("keycloak: get role %q failed: status %d: %s", role, resp.StatusCode, body)
	}

	var roleRealm struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&roleRealm); err != nil {
		return fmt.Errorf("keycloak: decode role: %w", err)
	}

	// 2. assign role to user
	assignURL := fmt.Sprintf("%s/admin/realms/%s/users/%s/role-mappings/realm", baseURL, realm, userID)

	payload := []map[string]string{
		{
			"id":   roleRealm.ID,
			"name": roleRealm.Name,
		},
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("keycloak: marshal assign role payload: %w", err)
	}

	assignReq, err := http.NewRequestWithContext(ctx, http.MethodPost, assignURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("keycloak: build assign role request: %w", err)
	}
	assignReq.Header.Set("Authorization", "Bearer "+token)
	assignReq.Header.Set("Content-Type", "application/json")

	assignResp, err := k.httpClient.Do(assignReq)
	if err != nil {
		return fmt.Errorf("keycloak: assign role request: %w", err)
	}
	defer assignResp.Body.Close()

	if assignResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(assignResp.Body)
		return fmt.Errorf("keycloak: assign role %q failed: status %d: %s", role, assignResp.StatusCode, body)
	}

	return nil
}

func (k *keycloakAdminClient) replaceBusinessRealmRoleForUser(ctx context.Context, userID string, newRole string) error {
	if strings.HasPrefix(userID, "test-kc-") {
		return nil
	}
	token, err := k.adminToken()
	if err != nil {
		return fmt.Errorf("keycloak: get admin token: %w", err)
	}

	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("keycloak: empty user id")
	}

	baseURL := k.baseURL()
	realm := k.realm()

	businessRoles := map[string]bool{
		string(authhandler.RoleSA):  true,
		string(authhandler.RoleADM): true,
		string(authhandler.RoleHR):  true,
		string(authhandler.RoleEMP): true,
	}
	if !businessRoles[newRole] {
		return fmt.Errorf("keycloak: unsupported business role %q", newRole)
	}

	// 1. Получаем все realm roles пользователя
	getAssignedURL := fmt.Sprintf("%s/admin/realms/%s/users/%s/role-mappings/realm", baseURL, realm, userID)

	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, getAssignedURL, nil)
	if err != nil {
		return fmt.Errorf("keycloak: build get assigned roles request: %w", err)
	}
	getReq.Header.Set("Authorization", "Bearer "+token)

	getResp, err := k.httpClient.Do(getReq)
	if err != nil {
		return fmt.Errorf("keycloak: get assigned roles request: %w", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getResp.Body)
		return fmt.Errorf("keycloak: get assigned roles failed: status %d: %s", getResp.StatusCode, body)
	}

	var assignedRoles []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&assignedRoles); err != nil {
		return fmt.Errorf("keycloak: decode assigned roles: %w", err)
	}

	// 2. Собираем старые бизнес-роли для удаления
	var rolesToRemove []map[string]string

	for _, r := range assignedRoles {

		if businessRoles[r.Name] {
			rolesToRemove = append(rolesToRemove, map[string]string{
				"id":   r.ID,
				"name": r.Name,
			})
		}
	}

	// 3. Удаляем старые бизнес-роли
	if len(rolesToRemove) > 0 {
		removeBody, err := json.Marshal(rolesToRemove)
		if err != nil {
			return fmt.Errorf("keycloak: marshal remove roles payload: %w", err)
		}

		removeReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, getAssignedURL, bytes.NewReader(removeBody))
		if err != nil {
			return fmt.Errorf("keycloak: build remove roles request: %w", err)
		}
		removeReq.Header.Set("Authorization", "Bearer "+token)
		removeReq.Header.Set("Content-Type", "application/json")

		removeResp, err := k.httpClient.Do(removeReq)
		if err != nil {
			return fmt.Errorf("keycloak: remove roles request: %w", err)
		}
		defer removeResp.Body.Close()

		if removeResp.StatusCode != http.StatusNoContent {
			body, _ := io.ReadAll(removeResp.Body)
			return fmt.Errorf("keycloak: remove roles failed: status %d: %s", removeResp.StatusCode, body)
		}
	}

	// 4. Если новая роль уже была среди назначенных, после удаления её надо назначить заново
	// Поэтому просто всегда назначаем новую роль
	roleURL := fmt.Sprintf("%s/admin/realms/%s/roles/%s", baseURL, realm, url.PathEscape(newRole))

	roleReq, err := http.NewRequestWithContext(ctx, http.MethodGet, roleURL, nil)
	if err != nil {
		return fmt.Errorf("keycloak: build get role request: %w", err)
	}
	roleReq.Header.Set("Authorization", "Bearer "+token)

	roleResp, err := k.httpClient.Do(roleReq)
	if err != nil {
		return fmt.Errorf("keycloak: get role request: %w", err)
	}
	defer roleResp.Body.Close()

	if roleResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(roleResp.Body)
		return fmt.Errorf("keycloak: get role %q failed: status %d: %s", newRole, roleResp.StatusCode, body)
	}

	var roleRealm struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(roleResp.Body).Decode(&roleRealm); err != nil {
		return fmt.Errorf("keycloak: decode role: %w", err)
	}

	assignPayload := []map[string]string{
		{
			"id":   roleRealm.ID,
			"name": roleRealm.Name,
		},
	}

	assignBody, err := json.Marshal(assignPayload)
	if err != nil {
		return fmt.Errorf("keycloak: marshal assign role payload: %w", err)
	}

	assignReq, err := http.NewRequestWithContext(ctx, http.MethodPost, getAssignedURL, bytes.NewReader(assignBody))
	if err != nil {
		return fmt.Errorf("keycloak: build assign role request: %w", err)
	}
	assignReq.Header.Set("Authorization", "Bearer "+token)
	assignReq.Header.Set("Content-Type", "application/json")

	assignResp, err := k.httpClient.Do(assignReq)
	if err != nil {
		return fmt.Errorf("keycloak: assign role request: %w", err)
	}
	defer assignResp.Body.Close()

	if assignResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(assignResp.Body)
		return fmt.Errorf("keycloak: assign role %q failed: status %d: %s", newRole, assignResp.StatusCode, body)
	}

	return nil
}

func (k *keycloakAdminClient) updateUserProfile(ctx context.Context, kcUserID string, email, dzoID, fullName *string, isActive *bool) error {
	if strings.HasPrefix(kcUserID, "test-kc-") {
		return nil
	}
	if strings.TrimSpace(kcUserID) == "" {
		return fmt.Errorf("keycloak: empty user id")
	}

	token, err := k.adminToken()
	if err != nil {
		return fmt.Errorf("keycloak: get admin token: %w", err)
	}

	userURL := fmt.Sprintf("%s/admin/realms/%s/users/%s", k.baseURL(), k.realm(), kcUserID)

	// Сначала читаем текущего пользователя, чтобы не затереть другие поля/attributes
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return fmt.Errorf("keycloak: build get user request: %w", err)
	}
	getReq.Header.Set("Authorization", "Bearer "+token)

	getResp, err := k.httpClient.Do(getReq)
	if err != nil {
		return fmt.Errorf("keycloak: get user request: %w", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getResp.Body)
		return fmt.Errorf("keycloak: get user failed: status %d: %s", getResp.StatusCode, body)
	}

	var current struct {
		Username   string              `json:"username"`
		Email      string              `json:"email"`
		FirstName  string              `json:"firstName"`
		LastName   string              `json:"lastName"`
		Enabled    bool                `json:"enabled"`
		Attributes map[string][]string `json:"attributes"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&current); err != nil {
		return fmt.Errorf("keycloak: decode user: %w", err)
	}

	if current.Attributes == nil {
		current.Attributes = map[string][]string{}
	}

	if email != nil {
		trimmedEmail := strings.TrimSpace(*email)
		current.Email = trimmedEmail
		current.Username = trimmedEmail
	}

	if dzoID != nil {
		current.Attributes["dzoId"] = []string{*dzoID}
	}
	if fullName != nil {
		firstName, lastName := splitFullName(*fullName)
		if firstName == "" {
			firstName = *fullName
		}
		if lastName == "" {
			lastName = *fullName
		}
		current.FirstName = firstName
		current.LastName = lastName
	}
	if isActive != nil {
		current.Enabled = *isActive
	}

	bodyBytes, err := json.Marshal(current)
	if err != nil {
		return fmt.Errorf("keycloak: marshal update user payload: %w", err)
	}

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, userURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("keycloak: build update user request: %w", err)
	}
	putReq.Header.Set("Authorization", "Bearer "+token)
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := k.httpClient.Do(putReq)
	if err != nil {
		return fmt.Errorf("keycloak: update user request: %w", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(putResp.Body)
		return fmt.Errorf("keycloak: update user failed: status %d: %s", putResp.StatusCode, body)
	}

	return nil
}

// deleteKeycloakUser удаляет пользователя из keycloak во время ошибок
func deleteKeycloakUser(ctx context.Context, kcUserID string) {
	if secrets.KeycloakIssuerURL == "" || kcUserID == "" || strings.HasPrefix(kcUserID, "stub-") {
		return
	}

	token, err := kcAdmin.adminToken()
	if err != nil {
		rlog.Error("compensation: failed to get admin token", "err", err.Error())
		return
	}

	userURL := fmt.Sprintf("%s/admin/realms/%s/users/%s",
		kcAdmin.baseURL(), kcAdmin.realm(), kcUserID)

	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, userURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := kcAdmin.httpClient.Do(req)
	if err != nil {
		rlog.Error("compensation: failed to delete keycloak user",
			"kcUserID", kcUserID, "err", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		rlog.Error("compensation: unexpected status on keycloak delete",
			"kcUserID", kcUserID, "status", resp.StatusCode, "body", string(body))
		return
	}
	rlog.Info("compensation: keycloak user deleted", "kcUserID", kcUserID)
}
func (k *keycloakAdminClient) handleUserConflict(
	ctx context.Context,
	email string,
) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	token, err := k.adminToken()
	if err != nil {
		return "", fmt.Errorf("keycloak: get admin token: %w", err)
	}

	// 1. найти пользователя по email
	searchURL := fmt.Sprintf(
		"%s/admin/realms/%s/users?email=%s&exact=true",
		k.baseURL(),
		k.realm(),
		url.QueryEscape(email),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("keycloak: search failed: %s", body)
	}

	var users []struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Enabled bool   `json:"enabled"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return "", err
	}

	if len(users) == 0 {
		return "", fmt.Errorf("keycloak: conflict but user not found")
	}

	user := users[0]

	// 2. если активный → конфликт
	if user.Enabled {
		return "", fmt.Errorf("keycloak: user with email %q already exists", email)
	}

	err = k.enableUser(ctx, user.ID, true)
	if err != nil {
		return "", err
	}

	err = k.setTemporaryPassword(ctx, user.ID, "da")
	if err != nil {
		return "", err
	}

	return user.ID, nil
}

func (k *keycloakAdminClient) enableUser(ctx context.Context, userID string, enabled bool) error {
	if strings.HasPrefix(userID, "test-kc-") {
		return nil
	}
	token, err := k.adminToken()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/admin/realms/%s/users/%s", k.baseURL(), k.realm(), userID)

	payload := map[string]any{
		"enabled": enabled,
	}

	body, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("enable user failed: %s", b)
	}

	return nil
}
func (k *keycloakAdminClient) setTemporaryPassword(ctx context.Context, userID, password string) error {
	token, err := k.adminToken()
	if err != nil {
		return err
	}

	url := fmt.Sprintf(
		"%s/admin/realms/%s/users/%s/reset-password",
		k.baseURL(),
		k.realm(),
		userID,
	)

	payload := map[string]any{
		"type":      "password",
		"value":     password,
		"temporary": true,
	}

	body, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := k.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("set password failed: %s", b)
	}

	return nil
}

func splitFullName(fullName string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(fullName))
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}
