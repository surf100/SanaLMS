package employees

import (
	"time"

	"encore.app/auth/authhandler"
	"github.com/google/uuid"
)

// Employee is the domain model representing a row in the employees table.
type Employee struct {
	ID            string               `json:"id"`
	ClientID      string               `json:"client_id"`
	DzoID         string               `json:"dzo_id"`
	DzoName       string               `json:"dzo_name"`
	Role          authhandler.UserRole `json:"role"`
	Position      *string              `json:"position,omitempty"`
	FullName      string               `json:"full_name"`
	ShortName     *string              `json:"short_name,omitempty"`
	Department    *string              `json:"department,omitempty"`
	Direction     *string              `json:"direction,omitempty"`
	Email         string               `json:"email"`
	InternalPhone *string              `json:"internal_phone,omitempty"`
	BirthDate     *string              `json:"birth_date,omitempty"`
	IsActive      bool                 `json:"is_active"`
	UserID        *string              `json:"user_id,omitempty"`
}

// CreateEmployeeRequest is the request body for creating a new employee.
type CreateEmployeeRequest struct {
	DzoID         string               `json:"dzo_id"`
	FullName      string               `json:"full_name"`
	Email         string               `json:"email"`
	Role          authhandler.UserRole `json:"role"`
	Position      *string              `json:"position,omitempty"`
	ShortName     *string              `json:"short_name,omitempty"`
	Department    *string              `json:"department,omitempty"`
	Direction     *string              `json:"direction,omitempty"`
	InternalPhone *string              `json:"internal_phone,omitempty"`
	BirthDate     *string              `json:"birth_date,omitempty"`
	UserID        *string              `json:"user_id,omitempty"`
}

// UpdateEmployeeRequest is the request body for partially updating an employee.
type UpdateEmployeeRequest struct {
	DzoID         *string               `json:"dzo_id,omitempty"`
	FullName      *string               `json:"full_name,omitempty"`
	Email         *string               `json:"email,omitempty"`
	Role          *authhandler.UserRole `json:"role,omitempty"`
	Position      *string               `json:"position,omitempty"`
	ShortName     *string               `json:"short_name,omitempty"`
	Department    *string               `json:"department,omitempty"`
	Direction     *string               `json:"direction,omitempty"`
	InternalPhone *string               `json:"internal_phone,omitempty"`
	BirthDate     *string               `json:"birth_date,omitempty"`
	IsActive      *bool                 `json:"is_active,omitempty"`
	UserID        *string               `json:"user_id,omitempty"`
}

// ListEmployeesParams holds optional query parameters for listing employees.
// Limit/Offset are used for lazy-load (infinite scroll) — Limit defaults to 20 (max 50).
type ListEmployeesParams struct {
	Search string `query:"search"`
	DzoID  string `query:"dzo_id"`
	Offset int    `query:"offset"`
	Page   int    `json:"page,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// GetEmployeeResponse is the response for fetching a single employee.
type GetEmployeeResponse struct {
	Employee Employee `json:"employee"`
}

// ListEmployeesResponse is the response for listing employees.
// Total is the number of employees matching the filters across all pages (unbounded by Limit/Offset).
// HasMore is true when Offset+len(Employees) < Total.
type ListEmployeesResponse struct {
	Employees  []Employee `json:"employees"`
	Total      int        `json:"total"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
	HasMore    bool       `json:"has_more"`
	Page       int        `json:"page"`
	TotalPages int        `json:"total_pages"`
}

// DeleteEmployeeResponse is the response for deleting an employee.
type DeleteEmployeeResponse struct {
	Message string `json:"message"`
}

// UploadEmployeesRequest is the request body for validating an uploaded employees .xlsx file.
type UploadEmployeesRequest struct {
	FileName string `json:"file_name"`
	FileData []byte `json:"file_data"`
}

// UploadResponse is the response for upload validation.
type UploadResponse struct {
	IsValid     bool                `json:"is_valid"`
	TotalRows   int                 `json:"total_rows"`
	ValidRows   int                 `json:"valid_rows"`
	InvalidRows int                 `json:"invalid_rows"`
	Errors      []string            `json:"errors"`
	Rows        []UploadEmployeeRow `json:"rows"`
}

// UploadEmployeeRow is a parsed preview row returned by the upload endpoint.
type UploadEmployeeRow struct {
	RowNumber     int      `json:"row_number"`
	DzoName       string   `json:"dzo_name"`
	FullName      string   `json:"full_name"`
	Email         string   `json:"email"`
	Position      *string  `json:"position,omitempty"`
	ShortName     *string  `json:"short_name,omitempty"`
	Department    *string  `json:"department,omitempty"`
	Direction     *string  `json:"direction,omitempty"`
	InternalPhone *string  `json:"internal_phone,omitempty"`
	BirthDate     *string  `json:"birth_date,omitempty"`
	UserID        *string  `json:"user_id,omitempty"`
	IsValid       bool     `json:"is_valid"`
	Include       bool     `json:"include"`
	Errors        []string `json:"errors"`
}

// ImportEmployeesRequest is the request body for importing employees from .xlsx file.
type ImportEmployeesRequest struct {
	FileName     string `json:"file_name"`
	FileData     []byte `json:"file_data"`
	SelectedRows []int  `json:"selected_rows,omitempty"`
}

// ImportResponse is the response for employee import.
type ImportResponse struct {
	ImportedCount int    `json:"imported_count"`
	Message       string `json:"message"`
}

type parsedEmployeeRow struct {
	DzoID         *uuid.UUID
	RowNumber     int
	DzoName       string
	FullName      string
	Email         string
	Position      *string
	ShortName     *string
	Department    *string
	Direction     *string
	InternalPhone *string
	BirthDate     *time.Time
	UserID        *uuid.UUID
}
type BulkDeleteRequest struct {
	IDs       []string `json:"ids,omitempty"`
	AllDzoIDs []string `json:"all_dzo_ids,omitempty"`
}

type BulkDeleteResponse struct {
	Message      string   `json:"message"`
	DeletedCount int      `json:"deleted_count"`
	Errors       []string `json:"errors,omitempty"`
}

type preparedImportRow struct {
	row   parsedEmployeeRow
	dzoID uuid.UUID
}

var employeeExcelRequiredHeaders = []string{"dzo_name", "full_name", "email"}

var employeeExcelHeaderAliases = map[string]string{
	"дзо":              "dzo_name",
	"полное_имя":       "full_name",
	"почта":            "email",
	"должность":        "position",
	"краткое_имя":      "short_name",
	"департамент":      "department",
	"направление":      "direction",
	"внутренний_номер": "internal_phone",
	"дата_рождения":    "birth_date",
	"идентификатор_пользователя": "user_id",

	"dzo_name":       "dzo_name",
	"full_name":      "full_name",
	"email":          "email",
	"position":       "position",
	"short_name":     "short_name",
	"department":     "department",
	"direction":      "direction",
	"internal_phone": "internal_phone",
	"birth_date":     "birth_date",
	"user_id":        "user_id",
}
