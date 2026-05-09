package requests

import (
	"time"

	"github.com/google/uuid"
)

type RequestType string

const (
	RequestTypeMain       RequestType = "MAIN"
	RequestTypeSubrequest RequestType = "SUBREQUEST"
)

func (t RequestType) IsValid() bool {
	switch t {
	case RequestTypeMain, RequestTypeSubrequest:
		return true
	default:
		return false
	}
}

type RequestKind string

const (
	RequestKindRegular  RequestKind = "REGULAR"
	RequestKindClosed   RequestKind = "CLOSED"
	RequestKindArchived RequestKind = "ARCHIVED"
)

func (k RequestKind) IsValid() bool {
	switch k {
	case RequestKindRegular, RequestKindClosed, RequestKindArchived:
		return true
	default:
		return false
	}
}

type RequestStatus string

const (
	RequestStatusDraft      RequestStatus = "DRAFT"
	RequestStatusInProgress RequestStatus = "IN_PROGRESS"
	RequestStatusPending    RequestStatus = "PENDING"
	RequestStatusApproved   RequestStatus = "APPROVED"
	RequestStatusRejected   RequestStatus = "REJECTED"
	RequestStatusCompleted  RequestStatus = "COMPLETED"
)

func (s RequestStatus) IsValid() bool {
	switch s {
	case RequestStatusDraft, RequestStatusInProgress, RequestStatusPending, RequestStatusApproved, RequestStatusRejected, RequestStatusCompleted:
		return true
	default:
		return false
	}
}

type CostMode string

const (
	CostModePerEmployee CostMode = "PER_EMPLOYEE"
	CostModeGroup       CostMode = "GROUP"
)

func (m CostMode) IsValid() bool {
	switch m {
	case CostModePerEmployee, CostModeGroup:
		return true
	default:
		return false
	}
}

// ════ ADMIN FLOW TYPES ════

type CreateAdminRequestRequest struct {
	TrainingEventID string    `json:"training_event_id"`
	Title           *string   `json:"title,omitempty"`
	Category        *string   `json:"category,omitempty"`
	Format          *string   `json:"format,omitempty"`
	EmployeeIDs     []string  `json:"employee_ids,omitempty"`
	DzoIDs          []string  `json:"dzo_ids,omitempty"`
	CostAmount      *float64  `json:"cost_amount,omitempty"`
	CostMode        *CostMode `json:"cost_mode,omitempty"`
	DeadlineAt      *string   `json:"deadline_at,omitempty"`
}

type ArchiveRequestContractInput struct {
	DzoID    string `json:"dzo_id"`
	FileName string `json:"file_name"`
	FileURL  string `json:"file_url"`
}

type CreateArchiveRequestRequest struct {
	Kind        RequestKind                   `json:"kind"`
	Title       *string                       `json:"title,omitempty"`
	Category    string                        `json:"category"`
	EmployeeIDs []string                      `json:"employee_ids"`
	Contracts   []ArchiveRequestContractInput `json:"contracts"`
}

type UpdateHRRequestEmployeesRequest struct {
	EmployeeIDs []string `json:"employee_ids"`
}

type RequestEmployee struct {
	ID       string `json:"id"`
	FullName string `json:"full_name"`
	DzoID    string `json:"dzo_id"`
	DzoName  string `json:"dzo_name"`
	IsActive bool   `json:"is_active"`
}

type RequestTargetDZO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RequestDzoContract struct {
	DzoID    string `json:"dzo_id"`
	FileName string `json:"file_name"`
	FileURL  string `json:"file_url"`
}

type RequestSummary struct {
	ID                  string        `json:"id"`
	InitiatorID         string        `json:"initiator_id"`
	ParentRequestID     *string       `json:"parent_request_id,omitempty"`
	TrainingEventID     string        `json:"training_event_id"`
	EntityType          string        `json:"entity_type"`
	RequestType         RequestType   `json:"request_type"`
	Kind                RequestKind   `json:"kind"`
	Status              RequestStatus `json:"status"`
	AssignedHRID        *string       `json:"assigned_hr_id,omitempty"`
	TargetDzoID         *string       `json:"target_dzo_id,omitempty"`
	Title               string        `json:"title"`
	Category            *string       `json:"category,omitempty"`
	Format              *string       `json:"format,omitempty"`
	ResponsibleAdminID  *string       `json:"responsible_admin_id,omitempty"`
	TrainingDate        *time.Time    `json:"training_date,omitempty"`
	DeadlineAt          *time.Time    `json:"deadline_at,omitempty"`
	CostAmount          *float64      `json:"cost_amount,omitempty"`
	CostMode            *CostMode     `json:"cost_mode,omitempty"`
	EmployeesCount      int           `json:"employees_count"`
	ApprovedChildren    int           `json:"approved_children"`
	TotalChildren       int           `json:"total_children"`
	IsBlocked           bool          `json:"is_blocked"`
	ReplacedByRequestID *string       `json:"replaced_by_request_id,omitempty"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
	CompletedAt         *time.Time    `json:"completed_at,omitempty"`
}

type RequestDetail struct {
	Request       RequestSummary       `json:"request"`
	Employees     []RequestEmployee    `json:"employees"`
	TargetDZOs    []RequestTargetDZO   `json:"target_dzos"`
	DZOContracts  []RequestDzoContract `json:"dzo_contracts"`
	ChildRequests []RequestSummary     `json:"child_requests"`
	Budget        *RequestBudgetInfo   `json:"budget,omitempty"`
	Supplier      *RequestSupplierInfo `json:"supplier,omitempty"`
}
type RequestBudgetInfo struct {
	ContractID      string  `json:"contract_id"`
	ContractNumber  string  `json:"contract_number"`
	Amount          float64 `json:"amount"`
	RemainingAmount float64 `json:"remaining_amount"`
	Currency        *string `json:"currency,omitempty"`
}

type RequestSupplierInfo struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	BinOrIin *string `json:"bin_or_iin,omitempty"`
}

type GetRequestResponse struct {
	Detail RequestDetail `json:"detail"`
}

type ListRequestsResponse struct {
	Items []RequestSummary `json:"items"`
}

// ════ BUDGET FLOW TYPES ════

type BudgetHistoryItem struct {
	OperationType string    `json:"operation_type"`
	Amount        float64   `json:"amount"`
	CreatedBy     string    `json:"created_by"`
	Reason        *string   `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}

type GetRequestBudgetHistoryResponse struct {
	Items []BudgetHistoryItem `json:"items"`
}

// CreateHRRequest creates a main request from HR to Admin.
type CreateHRRequestRequest struct {
	Title                  string   `json:"title"`
	EmployeeIDs            []string `json:"employee_ids,omitempty"`
	AllowInactiveEmployees bool     `json:"allow_inactive_employees,omitempty"`
	DeadlineAt             *string  `json:"deadline_at,omitempty"`
}

type PrepareAdminRequestRequest struct {
	TrainingEventID        string    `json:"training_event_id"`
	CostAmount             *float64  `json:"cost_amount,omitempty"`
	CostMode               *CostMode `json:"cost_mode,omitempty"`
	DeadlineAt             *string   `json:"deadline_at,omitempty"`
	AllowInactiveEmployees bool      `json:"allow_inactive_employees,omitempty"`
	ShowBudget             bool      `json:"show_budget,omitempty"`
}

type RemoveRequestEmployeeRequest struct {
	EmployeeID string `json:"employee_id"`
}

// ════ ARCHIVE FLOW TYPES ════

type CreateRequestRequest struct {
	EntityID   uuid.UUID `json:"entity_id"`
	EntityType string    `json:"entity_type"`
}

type UpdateRequestStepRequest struct {
	Step int `json:"step"`
}

type UpdateRequestStatusRequest struct {
	Status string `json:"status"`
}

type RequestContractResponse struct {
	DzoID    uuid.UUID `json:"dzo_id"`
	FileName string    `json:"file_name"`
	FileURL  string    `json:"file_url"`
}

type RequestResponse struct {
	ID          uuid.UUID                 `json:"id"`
	InitiatorID uuid.UUID                 `json:"initiator_id"`
	EntityID    uuid.UUID                 `json:"entity_id"`
	EntityType  string                    `json:"entity_type"`
	Kind        string                    `json:"kind"`
	Title       *string                   `json:"title,omitempty"`
	Category    *string                   `json:"category,omitempty"`
	Step        int                       `json:"step"`
	Status      string                    `json:"status"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
	CompletedAt *time.Time                `json:"completed_at,omitempty"`
	EmployeeIDs []uuid.UUID               `json:"employee_ids,omitempty"`
	Contracts   []RequestContractResponse `json:"contracts,omitempty"`
}
