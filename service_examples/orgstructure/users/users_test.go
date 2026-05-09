// Package users tests.
//
// This file imports encore.dev/storage/sqldb and cannot be run with plain go test.
// Use encore test ./orgstructure/users/... to run these tests.
package users

import (
	"testing"

	"encore.app/auth/authhandler"
)

func TestCanViewUsers_SA(t *testing.T) {
	if !CanViewUsers(authhandler.RoleSA) {
		t.Error("SA should be able to view users")
	}
}

func TestCanViewUsers_ADM(t *testing.T) {
	if !CanViewUsers(authhandler.RoleADM) {
		t.Error("ADM should be able to view users")
	}
}

func TestCanViewUsers_HR(t *testing.T) {
	if CanViewUsers(authhandler.RoleHR) {
		t.Error("HR should NOT be able to view users")
	}
}

func TestCanViewUsers_EMP(t *testing.T) {
	if CanViewUsers(authhandler.RoleEMP) {
		t.Error("EMP should NOT be able to view users")
	}
}

func TestCanManageUsers_SA(t *testing.T) {
	if !CanManageUsers(authhandler.RoleSA) {
		t.Error("SA should be able to manage users")
	}
}

func TestCanManageUsers_ADM(t *testing.T) {
	if !CanManageUsers(authhandler.RoleADM) {
		t.Error("ADM should be able to manage users")
	}
}

func TestCanManageUsers_HR(t *testing.T) {
	if CanManageUsers(authhandler.RoleHR) {
		t.Error("HR should NOT be able to manage users")
	}
}

func TestCanManageUsers_EMP(t *testing.T) {
	if CanManageUsers(authhandler.RoleEMP) {
		t.Error("EMP should NOT be able to manage users")
	}
}

func TestCanAssignRole_SA_AnyRole(t *testing.T) {
	for _, r := range []authhandler.UserRole{authhandler.RoleSA, authhandler.RoleADM, authhandler.RoleHR, authhandler.RoleEMP} {
		if !CanAssignRole(authhandler.RoleSA, r) {
			t.Errorf("SA should be able to assign role %q", r)
		}
	}
}

func TestCanAssignRole_ADM_HR(t *testing.T) {
	if !CanAssignRole(authhandler.RoleADM, authhandler.RoleHR) {
		t.Error("ADM should be able to assign HR role")
	}
}

func TestCanAssignRole_ADM_CannotAssignSA(t *testing.T) {
	if CanAssignRole(authhandler.RoleADM, authhandler.RoleSA) {
		t.Error("ADM should NOT be able to assign SA role")
	}
}

func TestCanAssignRole_ADM_CannotAssignADM(t *testing.T) {
	if CanAssignRole(authhandler.RoleADM, authhandler.RoleADM) {
		t.Error("ADM should NOT be able to assign ADM role")
	}
}

func TestCanAssignRole_HR_CannotAssignAny(t *testing.T) {
	for _, r := range []authhandler.UserRole{authhandler.RoleSA, authhandler.RoleADM, authhandler.RoleHR, authhandler.RoleEMP} {
		if CanAssignRole(authhandler.RoleHR, r) {
			t.Errorf("HR should NOT be able to assign role %q", r)
		}
	}
}

func TestCanAssignRole_EMP_CannotAssignAny(t *testing.T) {
	for _, r := range []authhandler.UserRole{authhandler.RoleSA, authhandler.RoleADM, authhandler.RoleHR, authhandler.RoleEMP} {
		if CanAssignRole(authhandler.RoleEMP, r) {
			t.Errorf("EMP should NOT be able to assign role %q", r)
		}
	}
}

func ptrStr(s string) *string { return &s }

// CanAccessUser — ADM is now scoped by CLIENT, not DZO.

func TestCanAccessUser_SA_AccessesAnyone(t *testing.T) {
	if !CanAccessUser(authhandler.RoleSA, ptrStr("client-1"), ptrStr("client-other")) {
		t.Error("SA should access any user regardless of client")
	}
}

func TestCanAccessUser_SA_AccessesUserWithoutClient(t *testing.T) {
	if !CanAccessUser(authhandler.RoleSA, nil, nil) {
		t.Error("SA should access user even without client")
	}
}

func TestCanAccessUser_ADM_SameClient(t *testing.T) {
	if !CanAccessUser(authhandler.RoleADM, ptrStr("client-1"), ptrStr("client-1")) {
		t.Error("ADM should access user in same client")
	}
}

func TestCanAccessUser_ADM_DifferentClient(t *testing.T) {
	if CanAccessUser(authhandler.RoleADM, ptrStr("client-1"), ptrStr("client-2")) {
		t.Error("ADM should NOT access user in different client")
	}
}

func TestCanAccessUser_ADM_TargetNoClient(t *testing.T) {
	if CanAccessUser(authhandler.RoleADM, ptrStr("client-1"), nil) {
		t.Error("ADM should NOT access user without client")
	}
}

func TestCanAccessUser_ADM_CallerNoClient(t *testing.T) {
	if CanAccessUser(authhandler.RoleADM, nil, ptrStr("client-1")) {
		t.Error("ADM without client should NOT access any user")
	}
}

func TestCanAccessUser_HR_Denied(t *testing.T) {
	if CanAccessUser(authhandler.RoleHR, ptrStr("client-1"), ptrStr("client-1")) {
		t.Error("HR should NOT access other users via CanAccessUser")
	}
}

func TestCanAccessUser_EMP_Denied(t *testing.T) {
	if CanAccessUser(authhandler.RoleEMP, ptrStr("client-1"), ptrStr("client-1")) {
		t.Error("EMP should NOT access other users")
	}
}

// CanUnblockUser tests.

func TestCanUnblockUser_SA(t *testing.T) {
	if !CanUnblockUser(authhandler.RoleSA) {
		t.Error("SA should be able to unblock users")
	}
}

func TestCanUnblockUser_ADM(t *testing.T) {
	if !CanUnblockUser(authhandler.RoleADM) {
		t.Error("ADM should be able to unblock users")
	}
}

func TestCanUnblockUser_HR(t *testing.T) {
	if CanUnblockUser(authhandler.RoleHR) {
		t.Error("HR should NOT be able to unblock users")
	}
}

func TestCanUnblockUser_EMP(t *testing.T) {
	if CanUnblockUser(authhandler.RoleEMP) {
		t.Error("EMP should NOT be able to unblock users")
	}
}

func TestCheckUserAccess_BlockedUserDenied(t *testing.T) {
	blocked := &User{IsActive: false}
	if err := CheckUserAccess(blocked); err == nil {
		t.Error("expected error for blocked user, got nil")
	}
}

func TestCheckUserAccess_ActiveUserAllowed(t *testing.T) {
	active := &User{IsActive: true}
	if err := CheckUserAccess(active); err != nil {
		t.Errorf("unexpected error for active user: %v", err)
	}
}

func TestAutoProvisionRole_AlwaysEMP(t *testing.T) {
	if got := AutoProvisionRole(); got != authhandler.RoleEMP {
		t.Errorf("AutoProvisionRole() = %q, want EMP", got)
	}
}

func TestRolePriority_SAIsHighest(t *testing.T) {
	if authhandler.RoleSA.Priority() <= authhandler.RoleADM.Priority() {
		t.Error("SA priority should be higher than ADM")
	}
}

func TestRolePriority_ADMOverHR(t *testing.T) {
	if authhandler.RoleADM.Priority() <= authhandler.RoleHR.Priority() {
		t.Error("ADM priority should be higher than HR")
	}
}

func TestRolePriority_HROverEMP(t *testing.T) {
	if authhandler.RoleHR.Priority() <= authhandler.RoleEMP.Priority() {
		t.Error("HR priority should be higher than EMP")
	}
}
