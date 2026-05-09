package dzo

import "time"

// DZO represents a subsidiary organization entity.
type DZO struct {
	ID             string    `json:"id"`
	ClientID       string    `json:"client_id"`
	Name           string    `json:"name"`
	ShortName      *string   `json:"short_name"`
	BIN            *string   `json:"bin"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	EmployeesCount int       `json:"employees_count"`
}

// CreateDZORequest is the request body for creating a new DZO.
type CreateDZORequest struct {
	ClientID  string  `json:"client_id"`
	Name      string  `json:"name"`
	ShortName *string `json:"short_name,omitempty"`
	BIN       *string `json:"bin,omitempty"`
}

// UpdateDZORequest is the request body for partially updating a DZO.
type UpdateDZORequest struct {
	Name      *string `json:"name,omitempty"`
	ShortName *string `json:"short_name,omitempty"`
	BIN       *string `json:"bin,omitempty"`
	IsActive  *bool   `json:"is_active,omitempty"`
}

// GetDZOResponse is the response for fetching a single DZO.
type GetDZOResponse struct {
	DZO DZO `json:"dzo"`
}

// ListDZOResponse is the response for listing DZO entities.
type ListDZOResponse struct {
	DZOs  []DZO `json:"dzos"`
	Total int   `json:"total"`
}

// DeleteDZOResponse is the response for deleting a DZO.
type DeleteDZOResponse struct {
	Message        string `json:"message"`
	EmployeesCount int    `json:"employees_count"`
}
