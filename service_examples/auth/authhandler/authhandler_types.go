package authhandler

import (
	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
)

// UserRole is a user role in the system.
type UserRole string

const (
	RoleSA  UserRole = "SA"
	RoleADM UserRole = "ADM"
	RoleHR  UserRole = "HR"
	RoleEMP UserRole = "EMP"
)

// IsValid reports whether the role is a known business role.
func (r UserRole) IsValid() bool {
	switch r {
	case RoleSA, RoleADM, RoleHR, RoleEMP:
		return true
	}
	return false
}

// Priority returns a numeric priority (higher = more privileged).
func (r UserRole) Priority() int {
	switch r {
	case RoleSA:
		return 4
	case RoleADM:
		return 3
	case RoleHR:
		return 2
	case RoleEMP:
		return 1
	default:
		return 0
	}
}

// AuthData holds the authenticated user's identity extracted from the JWT.
// Available via auth.Data() in every authenticated endpoint.
type AuthData struct {
	KeycloakUserID string   `json:"keycloak_user_id"`
	Email          string   `json:"email"`
	Role           UserRole `json:"role"`
	CompanyID      string   `json:"company_id"`
	DzoID          string   `json:"dzo_id"`
}

// RequirePermission extracts auth data and denies access for EMP role.
// Returns the auth data for SA, ADM, HR roles.
func RequirePermission() (*AuthData, error) {
	ad, ok := auth.Data().(*AuthData)
	if !ok {
		return nil, errs.B().Code(errs.PermissionDenied).Msg("Нет доступа").Err()
	}
	if ad.Role == RoleEMP {
		return nil, errs.B().Code(errs.PermissionDenied).Msg("Нет доступа").Err()
	}
	return ad, nil
}
func RequireMinRole(minRole UserRole) (*AuthData, error) {
	ad, err := RequirePermission()
	if err != nil {
		return nil, err
	}

	if ad.Role.Priority() < minRole.Priority() {
		return nil, errs.B().
			Code(errs.PermissionDenied).
			Msg("Недостаточно прав").
			Err()
	}

	return ad, nil
}
