package users

import (
	"context"
	"testing"

	"encore.dev/beta/errs"
	"github.com/google/uuid"

	"encore.app/auth/authhandler"
)

// ════ HELPERS ════

func ctx() context.Context {
	return context.Background()
}

func newID() string {
	return uuid.NewString()
}

func newEmail() string {
	return uuid.NewString() + "@example.com"
}

func validDzoID() string {
	return uuid.NewString()
}

func makeUser(t *testing.T, role authhandler.UserRole, dzoID *string) *User {
	t.Helper()

	resp, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: newID(),
		Email:          newEmail(),
		Role:           role,
		DzoID:          dzoID,
	})
	if err != nil {
		t.Fatalf("makeUser: %v", err)
	}
	return resp
}

// makeTestClientID creates a real Company row in the DB and returns its UUID string.
// Tests that set user.client_id need a valid FK row, so random UUIDs won't work.
func makeTestClientID(t *testing.T) string {
	t.Helper()
	c, err := Client.Company.
		Create().
		SetName("test-client-" + uuid.NewString()[:8]).
		Save(ctx())
	if err != nil {
		t.Fatalf("makeTestClientID: %v", err)
	}
	return c.ID.String()
}

// makePendingAdmin inserts a pending ADM record directly into the DB.
// ADM is scoped to a client, not a DZO.
func makePendingAdmin(t *testing.T) *User {
	t.Helper()

	resp, err := insertPendingAdmin(ctx(), newID(), &RegisterAdminRequest{
		Email:     newEmail(),
		FirstName: "Test",
		LastName:  "Admin",
		ClientID:  makeTestClientID(t),
	})
	if err != nil {
		t.Fatalf("makePendingAdmin: %v", err)
	}
	return resp
}

// ════ CREATE / INSERT ════

func TestInsertUser_Success(t *testing.T) {
	u, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: newID(),
		Email:          newEmail(),
		Role:           authhandler.RoleEMP,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID == "" {
		t.Error("expected non-empty ID")
	}
	if u.Role != authhandler.RoleEMP {
		t.Errorf("expected role EMP, got %q", u.Role)
	}
	if !u.IsActive {
		t.Error("expected user to be active by default")
	}
	if !u.IsOnboarded {
		t.Error("expected user to be onboarded by default")
	}
}

func TestInsertUser_SuccessWithDzo(t *testing.T) {
	dzoID := validDzoID()

	u, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: newID(),
		Email:          newEmail(),
		Role:           authhandler.RoleHR,
		DzoID:          &dzoID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.DzoID == nil || *u.DzoID != dzoID {
		t.Errorf("expected dzo_id %q, got %v", dzoID, u.DzoID)
	}
}

func TestInsertUser_InvalidDzoFormat(t *testing.T) {
	badDzoID := "not-a-uuid"

	_, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: newID(),
		Email:          newEmail(),
		Role:           authhandler.RoleEMP,
		DzoID:          &badDzoID,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestInsertUser_DuplicateKeycloakUserID(t *testing.T) {
	kcID := newID()

	_, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: kcID,
		Email:          newEmail(),
		Role:           authhandler.RoleEMP,
	})
	if err != nil {
		t.Fatalf("unexpected setup error: %v", err)
	}

	_, err = insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: kcID,
		Email:          newEmail(),
		Role:           authhandler.RoleEMP,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.AlreadyExists {
		t.Errorf("expected AlreadyExists, got %v", errs.Code(err))
	}
}

func TestInsertPendingAdmin_Success(t *testing.T) {
	clientID := makeTestClientID(t)

	u, err := insertPendingAdmin(ctx(), newID(), &RegisterAdminRequest{
		Email:     newEmail(),
		FirstName: "Test",
		LastName:  "Admin",
		ClientID:  clientID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Role != authhandler.RoleADM {
		t.Errorf("expected role ADM, got %q", u.Role)
	}
	if u.IsActive {
		t.Error("expected pending admin to be inactive")
	}
	if u.IsOnboarded {
		t.Error("expected pending admin to be non-onboarded")
	}
	// ADM is scoped to a client — dzo_id must be nil.
	if u.DzoID != nil {
		t.Errorf("expected dzo_id to be nil for ADM, got %v", u.DzoID)
	}
	if u.ClientID == nil || *u.ClientID != clientID {
		t.Errorf("expected client_id %q, got %v", clientID, u.ClientID)
	}
}

func TestInsertPendingAdmin_InvalidClientIDFormat(t *testing.T) {
	_, err := insertPendingAdmin(ctx(), newID(), &RegisterAdminRequest{
		Email:     newEmail(),
		FirstName: "Test",
		LastName:  "Admin",
		ClientID:  "not-a-uuid",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestInsertPendingAdmin_DuplicateKeycloakUserID(t *testing.T) {
	kcID := newID()

	_, err := insertPendingAdmin(ctx(), kcID, &RegisterAdminRequest{
		Email:     newEmail(),
		FirstName: "Test",
		LastName:  "Admin",
		ClientID:  makeTestClientID(t),
	})
	if err != nil {
		t.Fatalf("unexpected setup error: %v", err)
	}

	_, err = insertPendingAdmin(ctx(), kcID, &RegisterAdminRequest{
		Email:     newEmail(),
		FirstName: "Test2",
		LastName:  "Admin2",
		ClientID:  makeTestClientID(t),
	})
	if err == nil {
		t.Fatal("expected error for duplicate keycloak_user_id, got nil")
	}
	if errs.Code(err) != errs.AlreadyExists {
		t.Errorf("expected AlreadyExists, got %v", errs.Code(err))
	}
}

// ════ GET / QUERY ════

func TestQueryUserByID_Success(t *testing.T) {
	created := makeUser(t, authhandler.RoleEMP, nil)

	u, err := queryUserByID(ctx(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, u.ID)
	}
}

func TestQueryUserByID_InvalidIDFormat(t *testing.T) {
	_, err := queryUserByID(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestQueryUserByID_NotFound(t *testing.T) {
	_, err := queryUserByID(ctx(), "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestQueryUserByKeycloakID_Success(t *testing.T) {
	kcID := newID()

	created, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: kcID,
		Email:          newEmail(),
		Role:           authhandler.RoleEMP,
	})
	if err != nil {
		t.Fatalf("unexpected setup error: %v", err)
	}

	u, err := queryUserByKeycloakID(ctx(), kcID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, u.ID)
	}
}

func TestQueryUserByKeycloakID_NotFound(t *testing.T) {
	_, err := queryUserByKeycloakID(ctx(), newID())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

// ════ LIST ════

func TestQueryActiveUsers_ReturnsOnlyActiveUsers(t *testing.T) {
	active := makeUser(t, authhandler.RoleEMP, nil)
	inactive := makeUser(t, authhandler.RoleEMP, nil)

	if err := updateUserActive(ctx(), inactive.ID, false); err != nil {
		t.Fatalf("failed to deactivate user: %v", err)
	}

	users, err := queryActiveUsers(ctx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundActive := false
	for _, u := range users {
		if u.ID == inactive.ID {
			t.Error("inactive user should not appear in active users list")
		}
		if u.ID == active.ID {
			foundActive = true
		}
	}
	if !foundActive {
		t.Error("active user should appear in active users list")
	}
}

func TestQueryActiveUsersByDzo_ReturnsOnlyActiveUsersForDzo(t *testing.T) {
	dzoA := validDzoID()
	dzoB := validDzoID()

	a1 := makeUser(t, authhandler.RoleEMP, &dzoA)
	a2 := makeUser(t, authhandler.RoleHR, &dzoA)
	b1 := makeUser(t, authhandler.RoleEMP, &dzoB)

	if err := updateUserActive(ctx(), a2.ID, false); err != nil {
		t.Fatalf("failed to deactivate user: %v", err)
	}

	users, err := queryActiveUsersByDzo(ctx(), dzoA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundA1 := false
	for _, u := range users {
		if u.ID == a2.ID {
			t.Error("inactive DZO user should not appear in list")
		}
		if u.ID == b1.ID {
			t.Error("user from another DZO should not appear in list")
		}
		if u.ID == a1.ID {
			foundA1 = true
		}
	}
	if !foundA1 {
		t.Error("expected active same-DZO user to appear")
	}
}

func TestQueryActiveUsersByDzo_InvalidDzoFormat(t *testing.T) {
	_, err := queryActiveUsersByDzo(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

// ════ UPDATE ════

func TestUpdateUserRole_Success(t *testing.T) {
	u := makeUser(t, authhandler.RoleEMP, nil)
	dzoID := validDzoID()

	updated, err := updateUserRole(ctx(), u.ID, authhandler.RoleHR, &dzoID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Role != authhandler.RoleHR {
		t.Errorf("expected role HR, got %q", updated.Role)
	}
	if updated.DzoID == nil || *updated.DzoID != dzoID {
		t.Errorf("expected dzo_id %q, got %v", dzoID, updated.DzoID)
	}
}

func TestUpdateUserRole_InvalidIDFormat(t *testing.T) {
	_, err := updateUserRole(ctx(), "not-a-uuid", authhandler.RoleEMP, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateUserRole_InvalidDzoFormat(t *testing.T) {
	u := makeUser(t, authhandler.RoleEMP, nil)
	badDzoID := "not-a-uuid"

	_, err := updateUserRole(ctx(), u.ID, authhandler.RoleHR, &badDzoID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateUserRole_NotFound(t *testing.T) {
	_, err := updateUserRole(ctx(), "00000000-0000-0000-0000-000000000000", authhandler.RoleEMP, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestUpdateUserActive_Success(t *testing.T) {
	u := makeUser(t, authhandler.RoleEMP, nil)

	if err := updateUserActive(ctx(), u.ID, false); err != nil {
		t.Fatalf("unexpected error deactivating user: %v", err)
	}

	got, err := queryUserByID(ctx(), u.ID)
	if err != nil {
		t.Fatalf("unexpected error querying user: %v", err)
	}
	if got.IsActive {
		t.Error("expected user to be inactive")
	}

	if err := updateUserActive(ctx(), u.ID, true); err != nil {
		t.Fatalf("unexpected error activating user: %v", err)
	}

	got, err = queryUserByID(ctx(), u.ID)
	if err != nil {
		t.Fatalf("unexpected error querying user: %v", err)
	}
	if !got.IsActive {
		t.Error("expected user to be active")
	}
}

func TestUpdateUserActive_InvalidIDFormat(t *testing.T) {
	err := updateUserActive(ctx(), "not-a-uuid", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateUserActive_NotFound(t *testing.T) {
	err := updateUserActive(ctx(), "00000000-0000-0000-0000-000000000000", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

// ════ ONBOARDING / RESOLUTION ════

func TestActivateOnboarding_Success(t *testing.T) {
	pending := makePendingAdmin(t)

	activated, err := activateOnboarding(ctx(), pending.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !activated.IsActive {
		t.Error("expected user to be active after onboarding")
	}
	if !activated.IsOnboarded {
		t.Error("expected user to be onboarded after activation")
	}
}

func TestActivateOnboarding_InvalidIDFormat(t *testing.T) {
	_, err := activateOnboarding(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestActivateOnboarding_NotFound(t *testing.T) {
	_, err := activateOnboarding(ctx(), "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestResolveCurrentUser_AutoProvisionsEMP(t *testing.T) {
	ad := &authhandler.AuthData{
		KeycloakUserID: newID(),
		Email:          newEmail(),
		Role:           authhandler.RoleEMP,
	}

	u, err := resolveCurrentUser(ctx(), ad)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.KeycloakUserID != ad.KeycloakUserID {
		t.Errorf("expected keycloak_user_id %q, got %q", ad.KeycloakUserID, u.KeycloakUserID)
	}
	if u.Role != authhandler.RoleEMP {
		t.Errorf("expected role EMP, got %q", u.Role)
	}
}

func TestResolveCurrentUser_AutoProvisionsSA(t *testing.T) {
	ad := &authhandler.AuthData{
		KeycloakUserID: newID(),
		Email:          newEmail(),
		Role:           authhandler.RoleSA,
	}

	u, err := resolveCurrentUser(ctx(), ad)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Role != authhandler.RoleSA {
		t.Errorf("expected role SA, got %q", u.Role)
	}
}

func TestResolveCurrentUser_ActivatesPendingAdmin(t *testing.T) {
	clientID := makeTestClientID(t)
	kcUserID := newID()

	pending, err := insertPendingAdmin(ctx(), kcUserID, &RegisterAdminRequest{
		Email:     newEmail(),
		FirstName: "Test",
		LastName:  "Admin",
		ClientID:  clientID,
	})
	if err != nil {
		t.Fatalf("unexpected setup error: %v", err)
	}

	// Simulate first login: ADM presents their JWT, backend finds pending record and activates it.
	ad := &authhandler.AuthData{
		KeycloakUserID: pending.KeycloakUserID,
		Email:          pending.Email,
		Role:           authhandler.RoleADM,
		CompanyID:      clientID,
	}

	u, err := resolveCurrentUser(ctx(), ad)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !u.IsActive {
		t.Error("expected pending admin to become active on first login")
	}
	if !u.IsOnboarded {
		t.Error("expected pending admin to become onboarded on first login")
	}
	if u.DzoID != nil {
		t.Error("expected dzo_id to remain nil for ADM after activation")
	}
}

// ════ QUERY BY CLIENT ════

func TestQueryActiveUsersByClient_ReturnsOnlyClientUsers(t *testing.T) {
	clientA := makeTestClientID(t)
	clientB := makeTestClientID(t)

	a1, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: newID(),
		Email:          newEmail(),
		Role:           authhandler.RoleEMP,
		ClientID:       &clientA,
	})
	if err != nil {
		t.Fatalf("insertUser clientA: %v", err)
	}

	b1, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: newID(),
		Email:          newEmail(),
		Role:           authhandler.RoleEMP,
		ClientID:       &clientB,
	})
	if err != nil {
		t.Fatalf("insertUser clientB: %v", err)
	}

	users, err := queryActiveUsersByClient(ctx(), clientA)
	if err != nil {
		t.Fatalf("queryActiveUsersByClient: %v", err)
	}

	foundA1 := false
	for _, u := range users {
		if u.ID == b1.ID {
			t.Error("user from clientB must not appear in clientA results")
		}
		if u.ID == a1.ID {
			foundA1 = true
		}
	}
	if !foundA1 {
		t.Error("expected user from clientA to appear in results")
	}
}

func TestQueryActiveUsersByClient_ExcludesInactiveUsers(t *testing.T) {
	clientID := makeTestClientID(t)

	active, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: newID(),
		Email:          newEmail(),
		Role:           authhandler.RoleEMP,
		ClientID:       &clientID,
	})
	if err != nil {
		t.Fatalf("insertUser active: %v", err)
	}

	inactive, err := insertUser(ctx(), &CreateUserRequest{
		KeycloakUserID: newID(),
		Email:          newEmail(),
		Role:           authhandler.RoleEMP,
		ClientID:       &clientID,
	})
	if err != nil {
		t.Fatalf("insertUser inactive: %v", err)
	}
	if err := updateUserActive(ctx(), inactive.ID, false); err != nil {
		t.Fatalf("updateUserActive: %v", err)
	}

	users, err := queryActiveUsersByClient(ctx(), clientID)
	if err != nil {
		t.Fatalf("queryActiveUsersByClient: %v", err)
	}

	foundActive := false
	for _, u := range users {
		if u.ID == inactive.ID {
			t.Error("inactive user must not appear in active-only list")
		}
		if u.ID == active.ID {
			foundActive = true
		}
	}
	if !foundActive {
		t.Error("expected active user to appear in results")
	}
}

func TestQueryActiveUsersByClient_InvalidFormat(t *testing.T) {
	_, err := queryActiveUsersByClient(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

// ════ BLOCK / UNBLOCK ════

func TestCanUnblockUser_RoleCheck(t *testing.T) {
	cases := []struct {
		role authhandler.UserRole
		want bool
	}{
		{authhandler.RoleSA, true},
		{authhandler.RoleADM, true},
		{authhandler.RoleHR, false},
		{authhandler.RoleEMP, false},
	}
	for _, tc := range cases {
		got := CanUnblockUser(tc.role)
		if got != tc.want {
			t.Errorf("CanUnblockUser(%q) = %v, want %v", tc.role, got, tc.want)
		}
	}
}

// ADM cannot access a user from a different client.
func TestCanAccessUser_CrossClientDenied(t *testing.T) {
	clientA := uuid.NewString()
	clientB := uuid.NewString()
	if CanAccessUser(authhandler.RoleADM, &clientA, &clientB) {
		t.Error("ADM must not access user in a different client")
	}
}

// ADM can access a user within the same client.
func TestCanAccessUser_SameClientAllowed(t *testing.T) {
	clientID := uuid.NewString()
	if !CanAccessUser(authhandler.RoleADM, &clientID, &clientID) {
		t.Error("ADM must be able to access user in the same client")
	}
}

func TestResolveCurrentUser_BlockedUserDenied(t *testing.T) {
	u := makeUser(t, authhandler.RoleEMP, nil)

	if err := updateUserActive(ctx(), u.ID, false); err != nil {
		t.Fatalf("failed to block user: %v", err)
	}

	ad := &authhandler.AuthData{
		KeycloakUserID: u.KeycloakUserID,
		Email:          u.Email,
		Role:           u.Role,
	}

	_, err := resolveCurrentUser(ctx(), ad)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}