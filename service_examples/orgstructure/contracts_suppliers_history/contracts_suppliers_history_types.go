package contractsuppliershistory

import (
	"encoding/json"
	"time"
)

// OperationType represents the type of mutation that triggered an audit entry.
type OperationType string

const (
	OpCreate OperationType = "CREATE"
	OpUpdate OperationType = "UPDATE"
	OpDelete OperationType = "DELETE"
)

func (o OperationType) IsValid() bool {
	switch o {
	case OpCreate, OpUpdate, OpDelete:
		return true
	}
	return false
}

// ContractSupplier is the domain model for a contract-supplier row.
type ContractSupplier struct {
	ID                 string     `json:"id"`
	SupplierID         string     `json:"supplier_id"`
	ContractNumber     string     `json:"contract_number"`
	VatFlag            int        `json:"vat_flag"`
	SignedDate         time.Time  `json:"signed_date"`
	Amount             float64    `json:"amount"`
	AmountCurrency     *float64   `json:"amount_currency,omitempty"`
	Currency           *string    `json:"currency,omitempty"`
	BalanceAtYearEnd   *float64   `json:"balance_at_year_end,omitempty"`
	AmendmentNumber    *string    `json:"amendment_number,omitempty"`
	AmendmentDate      *time.Time `json:"amendment_date,omitempty"`
	AmendmentAmount    *float64   `json:"amendment_amount,omitempty"`
	TotalWithAmendment float64    `json:"total_with_amendment"`
	RemainingAmount    float64    `json:"remaining_amount"`
	IsActive           bool       `json:"is_active"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// HistoryRecord is a single audit entry returned by the history endpoint.
type HistoryRecord struct {
	HistoryID     string                 `json:"history_id"`
	ContractID    string                 `json:"contract_id"`
	OperationType OperationType          `json:"operation_type"`
	ChangedAt     time.Time              `json:"changed_at"`
	ChangedBy     *string                `json:"changed_by,omitempty"`
	Snapshot      json.RawMessage `json:"snapshot,omitempty"`
	Diff          json.RawMessage `json:"diff,omitempty"`
}

// ListHistoryResponse is the response for GET /contracts-suppliers/id/:id/history.
type ListHistoryResponse struct {
	Records []HistoryRecord `json:"records"`
	Total   int             `json:"total"`
}

// FieldDiff represents a single field change: { "old": X, "new": Y }.
type FieldDiff struct {
	Old json.RawMessage `json:"old"`
	New json.RawMessage `json:"new"`
}

// ValidationResult holds the outcome of a contract validation check.
type ValidationResult struct {
	IsValid bool     `json:"is_valid"`
	Errors  []string `json:"errors,omitempty"`
}

// ValidateResponse is the response for GET /contracts-suppliers/id/:id/validate.
type ValidateResponse struct {
	Result ValidationResult `json:"result"`
}
