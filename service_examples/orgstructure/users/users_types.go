package users

import (
	"time"

	"encore.app/auth/authhandler"
)

// User is the domain model representing a row in the users table.
type User struct {
	ID             string               `json:"id"`
	KeycloakUserID string               `json:"keycloak_user_id"`
	Email          string               `json:"email"`
	Role           authhandler.UserRole `json:"role"`
	DzoID          *string              `json:"dzo_id"`
	ClientID       *string              `json:"client_id,omitempty"`
	IsActive       bool                 `json:"is_active"`
	// IsOnboarded is false for admins registered via RegisterAdmin who have not yet
	// logged in for the first time.  It becomes true automatically on first login.
	// Regular users provisioned via auto-provision start as true.
	IsOnboarded bool      `json:"is_onboarded"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ════ REQUESTS ════

// CreateUserRequest is the request body for creating a new user (SA only).
type CreateUserRequest struct {
	KeycloakUserID string               `json:"keycloak_user_id"`
	Email          string               `json:"email"`
	Role           authhandler.UserRole `json:"role"`
	DzoID          *string              `json:"dzo_id,omitempty"`
	ClientID       *string              `json:"client_id,omitempty"`
}

// RegisterAdminRequest is the request body for registering a new admin.
// The backend creates the Keycloak account automatically — no pre-existing
// Keycloak user is required.
// ADM is scoped to an entire client (all DZOs inside it), so dzo_id is not
// needed here. DZO assignment is only relevant for HR.
type RegisterAdminRequest struct {
	Email    string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	// TempPassword is the initial password. If omitted, one is auto-generated
	// and returned in the response. Keycloak marks it as temporary so the
	// admin is forced to change it on first login.
	TempPassword *string `json:"temp_password,omitempty"`
	// ClientID is the client (company) this admin will manage. Required.
	ClientID string `json:"client_id"`
}

// RegisterAdminResponse wraps the created admin user and the temporary password.
// TempPassword is always returned so the SA can share it with the new admin.
type RegisterAdminResponse struct {
	User         User   `json:"user"`
	TempPassword string `json:"temp_password"`
}

// AssignRoleRequest is the request body for assigning a role (Flow 3).
type AssignRoleRequest struct {
	Role  authhandler.UserRole `json:"role"`
	DzoID *string              `json:"dzo_id,omitempty"`
}

// ════ RESPONSES ════

// GetUserResponse is the response for fetching a single user.
type GetUserResponse struct {
	User User `json:"user"`
}

// ListUsersResponse is the response for listing users.
type ListUsersResponse struct {
	Users []User `json:"users"`
	Total int    `json:"total"`
}

// MessageResponse is a generic message response.
type MessageResponse struct {
	Message string `json:"message"`
}
