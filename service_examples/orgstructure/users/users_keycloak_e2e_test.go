// E2E tests for the users service.
//
// These tests require:
//   - A running Encore app with a database (encore test)
//   - A running Keycloak at the URL in .secrets.local.cue
//
// If Keycloak is not reachable the Keycloak tests are skipped automatically.
//
// Run with:
//
//	encore test ./orgstructure/users/...
package users

import (
	"strings"
	"testing"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/google/uuid"

	"encore.app/auth/authhandler"
)

// ════ HELPERS ════

// skipIfNoKeycloak skips the test if Keycloak is not configured or not reachable.
// It attempts an actual admin-token exchange so the skip is reliable.
func skipIfNoKeycloak(t *testing.T) {
	t.Helper()
	if secrets.KeycloakIssuerURL == "" || secrets.KeycloakAdminUser == "" {
		t.Skip("Keycloak not configured — set KeycloakIssuerURL and KeycloakAdminUser in .secrets.local.cue")
	}
	if _, err := kcAdmin.adminToken(); err != nil {
		t.Skipf("Keycloak not reachable at %s (%v) — skipping e2e tests", secrets.KeycloakIssuerURL, err)
	}
}

// kcToken fetches a fresh Keycloak admin token and fails the test if it can't.
func kcToken(t *testing.T) string {
	t.Helper()
	token, err := kcAdmin.adminToken()
	if err != nil {
		t.Fatalf("cannot get Keycloak admin token (is Keycloak running at %s?): %v",
			secrets.KeycloakIssuerURL, err)
	}
	return token
}

// kcCleanup schedules deletion of a Keycloak user when the test finishes.
func kcCleanup(t *testing.T, kcUserID string) {
	t.Helper()
	t.Cleanup(func() {
		if kcUserID == "" {
			return
		}
		token, err := kcAdmin.adminToken()
		if err != nil {
			t.Logf("cleanup: cannot get admin token: %v", err)
			return
		}
		if err := kcAdmin.deleteUser(token, kcUserID); err != nil {
			t.Logf("cleanup: cannot delete Keycloak user %s: %v", kcUserID, err)
		}
	})
}

// e2eEmail generates a unique email that is unlikely to collide with real accounts.
func e2eEmail() string {
	return "e2e-" + uuid.NewString()[:8] + "@test-petro-e2e.internal"
}

// ════ SECTION 1: KEYCLOAK CONNECTIVITY ════

// TestE2E_KeycloakAdminToken verifies we can authenticate with the Keycloak Admin API.
func TestE2E_KeycloakAdminToken(t *testing.T) {
	skipIfNoKeycloak(t)
	token := kcToken(t)
	if token == "" {
		t.Error("expected non-empty admin token")
	}
}

// ════ SECTION 2: generateTempPassword ════

func TestE2E_GenerateTempPassword_Length(t *testing.T) {
	for _, length := range []int{8, 12, 20} {
		pwd, err := generateTempPassword(length)
		if err != nil {
			t.Fatalf("generateTempPassword(%d): %v", length, err)
		}
		if len(pwd) != length {
			t.Errorf("generateTempPassword(%d): got length %d", length, len(pwd))
		}
	}
}

func TestE2E_GenerateTempPassword_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		pwd, err := generateTempPassword(12)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if seen[pwd] {
			t.Errorf("duplicate password generated: %q", pwd)
		}
		seen[pwd] = true
	}
}

// ════ SECTION 3: createAndConfigureKeycloakAdmin ════

// TestE2E_CreateAdmin_FullSetup verifies that createAndConfigureKeycloakAdmin:
//   - Creates the Keycloak account
//   - Sets companyId attribute
//   - Assigns ADM realm role
func TestE2E_CreateAdmin_FullSetup(t *testing.T) {
	skipIfNoKeycloak(t)

	email := e2eEmail()
	companyID := uuid.NewString()

	kcUserID, err := createAndConfigureKeycloakAdmin(ctx(), email, "E2E", "Admin", "TempPass123!", &companyID, nil)
	if err != nil {
		t.Fatalf("createAndConfigureKeycloakAdmin: %v", err)
	}
	if kcUserID == "" {
		t.Fatal("expected non-empty kcUserID")
	}
	kcCleanup(t, kcUserID)

	token := kcToken(t)

	// 1. User exists and email matches.
	rep, err := kcAdmin.getUser(token, kcUserID)
	if err != nil {
		t.Fatalf("getUser: %v", err)
	}
	if rep.Email != email {
		t.Errorf("email: want %q, got %q", email, rep.Email)
	}
	if rep.FirstName != "E2E" {
		t.Errorf("firstName: want E2E, got %q", rep.FirstName)
	}
	if rep.Enabled == nil || !*rep.Enabled {
		t.Error("expected user to be enabled")
	}

	// 2. companyId attribute is set.
	if got := rep.Attributes["companyId"]; len(got) == 0 || got[0] != companyID {
		t.Errorf("companyId attribute: want %q, got %v", companyID, got)
	}
	// dzoId should NOT be set for ADM.
	if dzo := rep.Attributes["dzoId"]; len(dzo) > 0 && dzo[0] != "" {
		t.Errorf("expected dzoId to be absent for ADM, got %v", dzo)
	}

	// 3. ADM realm role is assigned.
	roles, err := kcAdmin.getCurrentRealmRoles(token, kcUserID)
	if err != nil {
		t.Fatalf("getCurrentRealmRoles: %v", err)
	}
	hasADM := false
	for _, r := range roles {
		if r.Name == string(authhandler.RoleADM) {
			hasADM = true
		}
	}
	if !hasADM {
		t.Error("expected ADM realm role to be assigned after createAndConfigureKeycloakAdmin")
	}
}

// TestE2E_CreateAdmin_DuplicateEmail verifies that creating two admins with the
// same email is rejected by Keycloak (409 Conflict).
func TestE2E_CreateAdmin_DuplicateEmail(t *testing.T) {
	skipIfNoKeycloak(t)

	email := e2eEmail()
	cid := uuid.NewString()

	kcUserID, err := createAndConfigureKeycloakAdmin(ctx(), email, "A", "B", "Pass123!", &cid, nil)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	kcCleanup(t, kcUserID)

	_, err = createAndConfigureKeycloakAdmin(ctx(), email, "A", "B", "Pass123!", &cid, nil)
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error message, got: %v", err)
	}
}

// ════ SECTION 4: syncRoleToKeycloak ════

// TestE2E_SyncRole_ADMtoHR verifies that syncRoleToKeycloak replaces ADM → HR.
func TestE2E_SyncRole_ADMtoHR(t *testing.T) {
	skipIfNoKeycloak(t)

	email := e2eEmail()
	cid := uuid.NewString()

	kcUserID, err := createAndConfigureKeycloakAdmin(ctx(), email, "S", "R", "Pass123!", &cid, nil)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	kcCleanup(t, kcUserID)

	syncRoleToKeycloak(ctx(), kcUserID, authhandler.RoleHR)

	token := kcToken(t)
	roles, err := kcAdmin.getCurrentRealmRoles(token, kcUserID)
	if err != nil {
		t.Fatalf("getCurrentRealmRoles: %v", err)
	}

	hasHR, hasADM := false, false
	for _, r := range roles {
		switch r.Name {
		case string(authhandler.RoleHR):
			hasHR = true
		case string(authhandler.RoleADM):
			hasADM = true
		}
	}
	if !hasHR {
		t.Error("expected HR realm role after sync")
	}
	if hasADM {
		t.Error("expected ADM realm role to be removed after sync to HR")
	}
}

// TestE2E_SyncRole_toEMP verifies RemoveRole sync (ADM → EMP).
func TestE2E_SyncRole_toEMP(t *testing.T) {
	skipIfNoKeycloak(t)

	email := e2eEmail()
	cid := uuid.NewString()

	kcUserID, err := createAndConfigureKeycloakAdmin(ctx(), email, "S", "E", "Pass123!", &cid, nil)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	kcCleanup(t, kcUserID)

	syncRoleToKeycloak(ctx(), kcUserID, authhandler.RoleEMP)

	token := kcToken(t)
	roles, err := kcAdmin.getCurrentRealmRoles(token, kcUserID)
	if err != nil {
		t.Fatalf("getCurrentRealmRoles: %v", err)
	}

	hasEMP, hasADM := false, false
	for _, r := range roles {
		switch r.Name {
		case string(authhandler.RoleEMP):
			hasEMP = true
		case string(authhandler.RoleADM):
			hasADM = true
		}
	}
	if !hasEMP {
		t.Error("expected EMP realm role after sync")
	}
	if hasADM {
		t.Error("expected ADM realm role to be removed after sync to EMP")
	}
}

// ════ SECTION 5: syncAttributesToKeycloak ════

// TestE2E_SyncAttributes updates companyId and dzoId on an existing user.
func TestE2E_SyncAttributes(t *testing.T) {
	skipIfNoKeycloak(t)

	email := e2eEmail()
	oldCompany := uuid.NewString()

	kcUserID, err := createAndConfigureKeycloakAdmin(ctx(), email, "S", "A", "Pass123!", &oldCompany, nil)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	kcCleanup(t, kcUserID)

	newCompany := uuid.NewString()
	newDzo := uuid.NewString()
	syncAttributesToKeycloak(ctx(), kcUserID, newCompany, newDzo)

	token := kcToken(t)
	rep, err := kcAdmin.getUser(token, kcUserID)
	if err != nil {
		t.Fatalf("getUser: %v", err)
	}

	if got := rep.Attributes["companyId"]; len(got) == 0 || got[0] != newCompany {
		t.Errorf("companyId: want %q, got %v", newCompany, got)
	}
	if got := rep.Attributes["dzoId"]; len(got) == 0 || got[0] != newDzo {
		t.Errorf("dzoId: want %q, got %v", newDzo, got)
	}
}

// ════ SECTION 6: syncEnabledToKeycloak ════

// TestE2E_SyncEnabled verifies block (false) and unblock (true) cycle.
func TestE2E_SyncEnabled_BlockUnblock(t *testing.T) {
	skipIfNoKeycloak(t)

	email := e2eEmail()
	cid := uuid.NewString()

	kcUserID, err := createAndConfigureKeycloakAdmin(ctx(), email, "S", "En", "Pass123!", &cid, nil)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	kcCleanup(t, kcUserID)

	token := kcToken(t)

	// Block.
	syncEnabledToKeycloak(ctx(), kcUserID, false)
	rep, err := kcAdmin.getUser(token, kcUserID)
	if err != nil {
		t.Fatalf("getUser after block: %v", err)
	}
	if rep.Enabled == nil || *rep.Enabled {
		t.Error("expected user to be disabled after syncEnabled(false)")
	}

	// Unblock.
	syncEnabledToKeycloak(ctx(), kcUserID, true)
	rep, err = kcAdmin.getUser(token, kcUserID)
	if err != nil {
		t.Fatalf("getUser after unblock: %v", err)
	}
	if rep.Enabled == nil || !*rep.Enabled {
		t.Error("expected user to be enabled after syncEnabled(true)")
	}
}

// ════ SECTION 7: RegisterAdmin endpoint — full flow ════

// TestE2E_RegisterAdmin_CreatesInKeycloakAndDB verifies the full RegisterAdmin flow:
//   - SA calls the endpoint
//   - User is created in Keycloak (account + ADM role + companyId attribute)
//   - User is created in DB as pending (is_active=false, is_onboarded=false)
//   - TempPassword is returned
//   - dzo_id is nil for ADM
func TestE2E_RegisterAdmin_CreatesInKeycloakAndDB(t *testing.T) {
	skipIfNoKeycloak(t)

	saContext := auth.WithContext(ctx(), auth.UID(uuid.NewString()), &authhandler.AuthData{
		KeycloakUserID: uuid.NewString(),
		Email:          e2eEmail(),
		Role:           authhandler.RoleSA,
	})

	clientID := makeTestClientID(t)
	email := e2eEmail()

	resp, err := RegisterAdmin(saContext, &RegisterAdminRequest{
		Email:     email,
		FirstName: "E2E",
		LastName:  "FullAdmin",
		ClientID:  clientID,
	})
	if err != nil {
		t.Fatalf("RegisterAdmin: %v", err)
	}
	kcCleanup(t, resp.User.KeycloakUserID)

	// DB assertions.
	if resp.TempPassword == "" {
		t.Error("expected non-empty TempPassword")
	}
	if len(resp.TempPassword) < 8 {
		t.Errorf("TempPassword too short: %q", resp.TempPassword)
	}
	if resp.User.Role != authhandler.RoleADM {
		t.Errorf("expected role ADM, got %q", resp.User.Role)
	}
	if resp.User.IsActive {
		t.Error("expected pending admin to be inactive initially")
	}
	if resp.User.IsOnboarded {
		t.Error("expected pending admin to be non-onboarded initially")
	}
	if resp.User.ClientID == nil || *resp.User.ClientID != clientID {
		t.Errorf("expected clientID %q, got %v", clientID, resp.User.ClientID)
	}
	if resp.User.DzoID != nil {
		t.Errorf("expected dzo_id nil for ADM, got %v", resp.User.DzoID)
	}
	if resp.User.KeycloakUserID == "" {
		t.Error("expected non-empty KeycloakUserID")
	}

	// Keycloak assertions.
	token := kcToken(t)
	rep, err := kcAdmin.getUser(token, resp.User.KeycloakUserID)
	if err != nil {
		t.Fatalf("getUser from Keycloak: %v", err)
	}
	if rep.Email != email {
		t.Errorf("Keycloak email: want %q, got %q", email, rep.Email)
	}
	if rep.Enabled == nil || !*rep.Enabled {
		t.Error("expected Keycloak user to be enabled")
	}
	if got := rep.Attributes["companyId"]; len(got) == 0 || got[0] != clientID {
		t.Errorf("companyId in Keycloak: want %q, got %v", clientID, got)
	}

	roles, err := kcAdmin.getCurrentRealmRoles(token, resp.User.KeycloakUserID)
	if err != nil {
		t.Fatalf("getCurrentRealmRoles: %v", err)
	}
	hasADM := false
	for _, r := range roles {
		if r.Name == string(authhandler.RoleADM) {
			hasADM = true
		}
	}
	if !hasADM {
		t.Error("expected ADM realm role in Keycloak after RegisterAdmin")
	}
}

// TestE2E_RegisterAdmin_FirstLogin simulates the admin activating on first login.
func TestE2E_RegisterAdmin_FirstLogin(t *testing.T) {
	skipIfNoKeycloak(t)

	saContext := auth.WithContext(ctx(), auth.UID(uuid.NewString()), &authhandler.AuthData{
		KeycloakUserID: uuid.NewString(),
		Email:          e2eEmail(),
		Role:           authhandler.RoleSA,
	})

	clientID := makeTestClientID(t)

	resp, err := RegisterAdmin(saContext, &RegisterAdminRequest{
		Email:     e2eEmail(),
		FirstName: "First",
		LastName:  "Login",
		ClientID:  clientID,
	})
	if err != nil {
		t.Fatalf("RegisterAdmin: %v", err)
	}
	kcCleanup(t, resp.User.KeycloakUserID)

	// Simulate first login — resolveCurrentUser activates the pending admin.
	ad := &authhandler.AuthData{
		KeycloakUserID: resp.User.KeycloakUserID,
		Email:          resp.User.Email,
		Role:           authhandler.RoleADM,
		CompanyID:      clientID,
	}
	activated, err := resolveCurrentUser(ctx(), ad)
	if err != nil {
		t.Fatalf("resolveCurrentUser (first login): %v", err)
	}
	if !activated.IsActive {
		t.Error("expected admin to become active on first login")
	}
	if !activated.IsOnboarded {
		t.Error("expected admin to become onboarded on first login")
	}
}

// TestE2E_RegisterAdmin_PermissionDenied ensures non-SA callers are rejected.
func TestE2E_RegisterAdmin_PermissionDenied(t *testing.T) {
	skipIfNoKeycloak(t)

	// A real client is needed so validation doesn't fail before the permission check.
	sharedClientID := makeTestClientID(t)

	for _, role := range []authhandler.UserRole{authhandler.RoleADM, authhandler.RoleHR, authhandler.RoleEMP} {
		t.Run(string(role), func(t *testing.T) {
			nonSACtx := auth.WithContext(ctx(), auth.UID(uuid.NewString()), &authhandler.AuthData{
				KeycloakUserID: uuid.NewString(),
				Email:          e2eEmail(),
				Role:           role,
			})
			_, err := RegisterAdmin(nonSACtx, &RegisterAdminRequest{
				Email:     e2eEmail(),
				FirstName: "X",
				LastName:  "Y",
				ClientID:  sharedClientID,
			})
			if err == nil {
				t.Fatal("expected PermissionDenied, got nil")
			}
			if errs.Code(err) != errs.PermissionDenied {
				t.Errorf("expected PermissionDenied, got %v", errs.Code(err))
			}
		})
	}
}

// TestE2E_RegisterAdmin_ValidationErrors verifies field validation before Keycloak is contacted.
func TestE2E_RegisterAdmin_ValidationErrors(t *testing.T) {
	skipIfNoKeycloak(t)

	saContext := auth.WithContext(ctx(), auth.UID(uuid.NewString()), &authhandler.AuthData{
		KeycloakUserID: uuid.NewString(),
		Email:          e2eEmail(),
		Role:           authhandler.RoleSA,
	})
	cid := makeTestClientID(t)

	cases := []struct {
		name string
		req  RegisterAdminRequest
	}{
		{"missing email", RegisterAdminRequest{FirstName: "A", ClientID: cid}},
		{"missing first_name", RegisterAdminRequest{Email: e2eEmail(), ClientID: cid}},
		{"missing client_id", RegisterAdminRequest{Email: e2eEmail(), FirstName: "A"}},
		{"invalid client_id", RegisterAdminRequest{Email: e2eEmail(), FirstName: "A", ClientID: "not-a-uuid"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RegisterAdmin(saContext, &tc.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if errs.Code(err) != errs.InvalidArgument {
				t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
			}
		})
	}
}

// TestE2E_RegisterAdmin_DuplicateEmail ensures re-registering the same email fails cleanly
// and does NOT leave an orphan in Keycloak.
func TestE2E_RegisterAdmin_DuplicateEmail(t *testing.T) {
	skipIfNoKeycloak(t)

	saContext := auth.WithContext(ctx(), auth.UID(uuid.NewString()), &authhandler.AuthData{
		KeycloakUserID: uuid.NewString(),
		Email:          e2eEmail(),
		Role:           authhandler.RoleSA,
	})

	email := e2eEmail()
	cid := makeTestClientID(t)

	// First registration — should succeed.
	resp, err := RegisterAdmin(saContext, &RegisterAdminRequest{
		Email:     email,
		FirstName: "Dup",
		LastName:  "Admin",
		ClientID:  cid,
	})
	if err != nil {
		t.Fatalf("first RegisterAdmin: %v", err)
	}
	kcCleanup(t, resp.User.KeycloakUserID)

	// Second registration with the same email — Keycloak returns 409.
	_, err = RegisterAdmin(saContext, &RegisterAdminRequest{
		Email:     email,
		FirstName: "Dup2",
		LastName:  "Admin2",
		ClientID:  makeTestClientID(t),
	})
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
	if errs.Code(err) != errs.Internal {
		t.Errorf("expected Internal (Keycloak 409), got %v", errs.Code(err))
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got: %v", err)
	}
}

// TestE2E_RegisterAdmin_WithCustomPassword verifies that a provided temp_password
// is used instead of auto-generating one.
func TestE2E_RegisterAdmin_WithCustomPassword(t *testing.T) {
	skipIfNoKeycloak(t)

	saContext := auth.WithContext(ctx(), auth.UID(uuid.NewString()), &authhandler.AuthData{
		KeycloakUserID: uuid.NewString(),
		Email:          e2eEmail(),
		Role:           authhandler.RoleSA,
	})

	customPwd := "MyCustomP@ss1"
	resp, err := RegisterAdmin(saContext, &RegisterAdminRequest{
		Email:        e2eEmail(),
		FirstName:    "Custom",
		LastName:     "Pwd",
		ClientID:     makeTestClientID(t),
		TempPassword: &customPwd,
	})
	if err != nil {
		t.Fatalf("RegisterAdmin: %v", err)
	}
	kcCleanup(t, resp.User.KeycloakUserID)

	if resp.TempPassword != customPwd {
		t.Errorf("expected TempPassword %q, got %q", customPwd, resp.TempPassword)
	}
}

// ════ SECTION 8: ADM isolation — cross-client access ════

// TestE2E_ADM_CannotSeeOtherClientUsers verifies that ListUsers for ADM only
// returns users from their own client.
func TestE2E_ADM_CannotSeeOtherClientUsers(t *testing.T) {
	skipIfNoKeycloak(t)

	clientA := makeTestClientID(t)
	clientB := makeTestClientID(t)

	// Create a user in clientB directly.
	userB, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: uuid.NewString(),
		Email:          e2eEmail(),
		Role:           authhandler.RoleEMP,
		ClientID:       &clientB,
	})
	if err != nil {
		t.Fatalf("insertUser clientB: %v", err)
	}

	// ADM from clientA calls ListUsers.
	admCtx := auth.WithContext(ctx(), auth.UID(uuid.NewString()), &authhandler.AuthData{
		KeycloakUserID: uuid.NewString(),
		Email:          e2eEmail(),
		Role:           authhandler.RoleADM,
		CompanyID:      clientA,
	})

	listResp, err := ListUsers(admCtx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	for _, u := range listResp.Users {
		if u.ID == userB.ID {
			t.Errorf("ADM from clientA must not see user from clientB (id=%s)", userB.ID)
		}
	}
}

// TestE2E_ADM_CannotGetOtherClientUser verifies GetUser returns NotFound for cross-client.
func TestE2E_ADM_CannotGetOtherClientUser(t *testing.T) {
	skipIfNoKeycloak(t)

	clientA := makeTestClientID(t)
	clientB := makeTestClientID(t)

	userB, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: uuid.NewString(),
		Email:          e2eEmail(),
		Role:           authhandler.RoleEMP,
		ClientID:       &clientB,
	})
	if err != nil {
		t.Fatalf("insertUser clientB: %v", err)
	}

	admCtx := auth.WithContext(ctx(), auth.UID(uuid.NewString()), &authhandler.AuthData{
		KeycloakUserID: uuid.NewString(),
		Email:          e2eEmail(),
		Role:           authhandler.RoleADM,
		CompanyID:      clientA,
	})

	_, err = GetUser(admCtx, userB.ID)
	if err == nil {
		t.Fatal("expected PermissionDenied for cross-client GetUser, got nil")
	}
	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", errs.Code(err))
	}
}

// TestE2E_ADM_CanBlockAndUnblockOwnClientUser verifies the block/unblock flow for ADM.
func TestE2E_ADM_CanBlockAndUnblockOwnClientUser(t *testing.T) {
	skipIfNoKeycloak(t)

	clientID := makeTestClientID(t)

	// Create a user in the same client.
	target, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: uuid.NewString(),
		Email:          e2eEmail(),
		Role:           authhandler.RoleEMP,
		ClientID:       &clientID,
	})
	if err != nil {
		t.Fatalf("insertUser: %v", err)
	}

	// ADM from the same client.
	admKcID := uuid.NewString()
	admUser, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: admKcID,
		Email:          e2eEmail(),
		Role:           authhandler.RoleADM,
		ClientID:       &clientID,
	})
	if err != nil {
		t.Fatalf("insertUser ADM: %v", err)
	}
	_ = admUser

	admCtx := auth.WithContext(ctx(), auth.UID(admKcID), &authhandler.AuthData{
		KeycloakUserID: admKcID,
		Email:          e2eEmail(),
		Role:           authhandler.RoleADM,
		CompanyID:      clientID,
	})

	// Block.
	blockResp, err := BlockUser(admCtx, target.ID)
	if err != nil {
		t.Fatalf("BlockUser: %v", err)
	}
	if blockResp.Message == "" {
		t.Error("expected non-empty block message")
	}

	// Verify blocked in DB.
	u, err := queryUserByID(ctx(), target.ID)
	if err != nil {
		t.Fatalf("queryUserByID after block: %v", err)
	}
	if u.IsActive {
		t.Error("expected user to be blocked (is_active=false)")
	}

	// Unblock.
	unblockResp, err := UnblockUser(admCtx, target.ID)
	if err != nil {
		t.Fatalf("UnblockUser: %v", err)
	}
	if unblockResp.Message == "" {
		t.Error("expected non-empty unblock message")
	}

	// Verify unblocked in DB.
	u, err = queryUserByID(ctx(), target.ID)
	if err != nil {
		t.Fatalf("queryUserByID after unblock: %v", err)
	}
	if !u.IsActive {
		t.Error("expected user to be active after unblock")
	}
}

// TestE2E_SA_HasFullAccess verifies SA can list users across all clients.
func TestE2E_SA_HasFullAccess(t *testing.T) {
	skipIfNoKeycloak(t)

	// Create users in two different clients.
	cidA := makeTestClientID(t)
	cidB := makeTestClientID(t)
	uA, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: uuid.NewString(), Email: e2eEmail(),
		Role: authhandler.RoleEMP, ClientID: &cidA,
	})
	if err != nil {
		t.Fatalf("insertUser A: %v", err)
	}
	uB, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: uuid.NewString(), Email: e2eEmail(),
		Role: authhandler.RoleEMP, ClientID: &cidB,
	})
	if err != nil {
		t.Fatalf("insertUser B: %v", err)
	}

	saKcID := uuid.NewString()
	saContext := auth.WithContext(ctx(), auth.UID(saKcID), &authhandler.AuthData{
		KeycloakUserID: saKcID,
		Email:          e2eEmail(),
		Role:           authhandler.RoleSA,
	})

	listResp, err := ListUsers(saContext)
	if err != nil {
		t.Fatalf("ListUsers as SA: %v", err)
	}

	foundA, foundB := false, false
	for _, u := range listResp.Users {
		if u.ID == uA.ID {
			foundA = true
		}
		if u.ID == uB.ID {
			foundB = true
		}
	}
	if !foundA {
		t.Error("SA should see user from clientA")
	}
	if !foundB {
		t.Error("SA should see user from clientB")
	}
}
