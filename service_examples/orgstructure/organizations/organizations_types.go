package organizations

import "time"

// OrgType represents the type of an organization.
type OrgType string

const (
	OrgTypeCompany    OrgType = "company"
	OrgTypeSubsidiary OrgType = "subsidiary"
	OrgTypeDepartment OrgType = "department"
	OrgTypeDivision   OrgType = "division"
)

func (o OrgType) IsValid() bool {
	switch o {
	case OrgTypeCompany, OrgTypeSubsidiary, OrgTypeDepartment, OrgTypeDivision:
		return true
	}
	return false
}

// Organization is the domain model representing a row in the organizations table.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	ParentID  *string   `json:"parent_id"`
	Type      OrgType   `json:"type"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateOrgRequest is the request body for creating a new organization.
type CreateOrgRequest struct {
	Name     string  `json:"name"`
	Code     string  `json:"code"`
	ParentID *string `json:"parent_id,omitempty"`
	Type     OrgType `json:"type"`
}

// UpdateOrgRequest is the request body for partially updating an organization.
type UpdateOrgRequest struct {
	Name     *string  `json:"name,omitempty"`
	Code     *string  `json:"code,omitempty"`
	ParentID *string  `json:"parent_id,omitempty"`
	Type     *OrgType `json:"type,omitempty"`
	IsActive *bool    `json:"is_active,omitempty"`
}

// GetOrgResponse is the response for fetching a single organization.
type GetOrgResponse struct {
	Organization Organization `json:"organization"`
}

// ListOrgsResponse is the response for listing organizations.
type ListOrgsResponse struct {
	Organizations []Organization `json:"organizations"`
	Total         int            `json:"total"`
}

// DeleteOrgResponse is the response for deleting an organization.
type DeleteOrgResponse struct {
	Message string `json:"message"`
}
