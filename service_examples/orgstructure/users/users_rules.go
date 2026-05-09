package users

import (
	"errors"

	"encore.dev/beta/errs"

	"encore.app/auth/authhandler"
)

func CanViewUsers(role authhandler.UserRole) bool {
	return role == authhandler.RoleSA || role == authhandler.RoleADM
}

func CanManageUsers(role authhandler.UserRole) bool {
	return role == authhandler.RoleSA || role == authhandler.RoleADM
}

// CanAssignRole returns true if callerRole may assign targetRole.
// SA can assign any role; ADM can only assign HR.
func CanAssignRole(callerRole, targetRole authhandler.UserRole) bool {
	if callerRole == authhandler.RoleSA {
		return true
	}
	if callerRole == authhandler.RoleADM && targetRole == authhandler.RoleHR {
		return true
	}
	return false
}

// CanAccessUser returns true if the caller may read or modify the target.
// SA has unrestricted access; ADM is scoped to their own client (company).
func CanAccessUser(callerRole authhandler.UserRole, callerClientID, targetClientID *string) bool {
	if callerRole == authhandler.RoleSA {
		return true
	}
	if callerRole == authhandler.RoleADM {
		if callerClientID == nil || targetClientID == nil {
			return false
		}
		return *callerClientID == *targetClientID
	}
	return false
}

// CanUnblockUser returns true if the role may unblock a user.
// Both SA and ADM can unblock; ADM is further scoped by their client at the call site.
func CanUnblockUser(role authhandler.UserRole) bool {
	return role == authhandler.RoleSA || role == authhandler.RoleADM
}

// AutoProvisionRole returns RoleEMP. Trusting JWT claims for provisioning would
// allow privilege escalation; explicit elevation must go through AssignRole.
func AutoProvisionRole() authhandler.UserRole {
	return authhandler.RoleEMP
}

// IsPendingActivation returns true for admins registered by SA who have not
// yet logged in. State matrix:
//
//	is_onboarded=false, is_active=false → PENDING  (auto-activate on first login)
//	is_onboarded=true,  is_active=true  → ACTIVE
//	is_onboarded=true,  is_active=false → BLOCKED
func IsPendingActivation(isOnboarded, isActive bool) bool {
	return !isOnboarded && !isActive
}

// ErrUserBlocked is returned by checkUserAccess when the user is blocked.
var ErrUserBlocked = errors.New("user is blocked")

// CheckUserAccess returns PermissionDenied when the user is blocked.
func CheckUserAccess(u *User) error {
	if !u.IsActive {
		return errs.B().Code(errs.PermissionDenied).Msg(ErrUserBlocked.Error()).Err()
	}
	return nil
}
