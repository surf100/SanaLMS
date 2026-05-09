package suppliers

type SupplierType string

const (
	SupplierTypeLegal      SupplierType = "LEGAL"
	SupplierTypeIndividual SupplierType = "INDIVIDUAL"
)

func (s SupplierType) IsValid() bool {
	return s == SupplierTypeLegal || s == SupplierTypeIndividual
}

// Supplier — supplier domain model
type Supplier struct {
	ID              string       `json:"id"`
	ClientID        *string      `json:"client_id"`
	Type            SupplierType `json:"type"`
	Name            string       `json:"name"`
	BinOrIIN        *string      `json:"bin_or_iin,omitempty"`
	LocalContentPct *float64     `json:"local_content_pct,omitempty"`
	IsActive        bool         `json:"is_active"`
}

// CreateSupplierRequest — body of the request to create a supplier
type CreateSupplierRequest struct {
	Type            SupplierType `json:"type"`
	Name            string       `json:"name"`
	BinOrIIN        *string      `json:"bin_or_iin,omitempty"`
	LocalContentPct *float64     `json:"local_content_pct,omitempty"`
}

// UpdateSupplierRequest — partial provider update request body.
type UpdateSupplierRequest struct {
	Name            *string       `json:"name,omitempty"`
	Type            *SupplierType `json:"type,omitempty"`
	BinOrIIN        *string       `json:"bin_or_iin,omitempty"`
	LocalContentPct *float64      `json:"local_content_pct,omitempty"`
	IsActive        *bool         `json:"is_active"`
}

// ListSuppliersParams — query parameters for filtering the supplier list.
// All fields are optional; omitting a field means no filter is applied for it.
// is_active defaults to true when not provided.
type ListSuppliersParams struct {
	Type     string `query:"type"`
	IsActive string `query:"is_active"`
	Search   string `query:"search"`
}

// GetSupplierResponse — response to receiving one supplier.
type GetSupplierResponse struct {
	Supplier Supplier `json:"supplier"`
}

// ListSuppliersResponse — response to receiving a list of suppliers.
type ListSuppliersResponse struct {
	Suppliers []Supplier `json:"suppliers"`
}

// DeleteSupplierResponse — response to supplier deletion.
type DeleteSupplierResponse struct {
	Message string `json:"message"`
}

// UploadSuppliersRequest — request body for validating an uploaded suppliers file.
type UploadSuppliersRequest struct {
	FileName string `json:"file_name"`
	FileData []byte `json:"file_data"`
}

// UploadSupplierRow — parsed preview row returned by the upload endpoint.
type UploadSupplierRow struct {
	RowNumber       int      `json:"row_number"`
	Type            string   `json:"type"`
	Name            string   `json:"name"`
	BinOrIIN        *string  `json:"bin_or_iin,omitempty"`
	LocalContentPct *float64 `json:"local_content_pct,omitempty"`
	IsValid         bool     `json:"is_valid"`
	Include         bool     `json:"include"`
	Errors          []string `json:"errors"`
}

// UploadSuppliersResponse — response for upload validation.
type UploadSuppliersResponse struct {
	IsValid     bool                `json:"is_valid"`
	TotalRows   int                 `json:"total_rows"`
	ValidRows   int                 `json:"valid_rows"`
	InvalidRows int                 `json:"invalid_rows"`
	Errors      []string            `json:"errors"`
	Rows        []UploadSupplierRow `json:"rows"`
}

// ImportSuppliersRequest — request body for importing suppliers from file.
type ImportSuppliersRequest struct {
	FileName     string `json:"file_name"`
	FileData     []byte `json:"file_data"`
	SelectedRows []int  `json:"selected_rows,omitempty"`
}

// ImportSuppliersResponse — response for supplier import.
type ImportSuppliersResponse struct {
	ImportedCount int    `json:"imported_count"`
	Message       string `json:"message"`
}

// parsedSupplierRow — internal parsed row, not exposed via API.
type parsedSupplierRow struct {
	RowNumber       int
	Type            SupplierType
	Name            string
	BinOrIIN        *string
	LocalContentPct *float64
}

// SupplierWithBudget — supplier with aggregated budget data from contract_suppliers.
type SupplierWithBudget struct {
	ID              string       `json:"id"`
	ClientID        *string      `json:"client_id"`
	Type            SupplierType `json:"type"`
	Name            string       `json:"name"`
	BinOrIIN        *string      `json:"bin_or_iin,omitempty"`
	LocalContentPct *float64     `json:"local_content_pct,omitempty"`
	IsActive        bool         `json:"is_active"`
	BudgetTotal     float64      `json:"budget_total"`
	BudgetUsed      float64      `json:"budget_used"`
	BudgetRemaining float64      `json:"budget_remaining"`
}

// ListSuppliersWithBudgetResponse — response for supplier list with budget aggregation.
type ListSuppliersWithBudgetResponse struct {
	Suppliers []SupplierWithBudget `json:"suppliers"`
}
