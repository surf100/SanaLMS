package external_trainings

import "time"

type CreateExternalTrainingRequest struct {
	Name              string    `json:"name"`
	CategoryID        string    `json:"category_id"`
	Format            string    `json:"format"`
	Capacity          int       `json:"capacity"`
	SupplierID        string    `json:"supplier_id"`
	ContractID        string    `json:"contract_id"`
	SupplierCostVAT   float64   `json:"supplier_cost_vat"`
	StartDate         time.Time `json:"start_date"`
	ResponsibleUserID string    `json:"responsible_user_id"`
}

type UpdateExternalTrainingRequest struct {
	Name              *string    `json:"name,omitempty"`
	CategoryID        *string    `json:"category_id,omitempty"`
	Format            *string    `json:"format,omitempty"`
	Capacity          *int       `json:"capacity,omitempty"`
	SupplierCostVAT   *float64   `json:"supplier_cost_vat,omitempty"`
	StartDate         *time.Time `json:"start_date,omitempty"`
	ResponsibleUserID *string    `json:"responsible_user_id,omitempty"`
}

type ExternalTrainingResponse struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	CategoryID        string    `json:"category_id,omitempty"`
	Format            string    `json:"format"`
	Capacity          int       `json:"capacity"`
	SupplierID        string    `json:"supplier_id"`
	ContractID        string    `json:"contract_id"`
	SupplierCostVAT   *float64  `json:"supplier_cost_vat,omitempty"`
	StartDate         time.Time `json:"start_date"`
	ResponsibleUserID string    `json:"responsible_user_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	IsDeleted         bool      `json:"is_deleted"`
}

type ListExternalTrainingsResponse struct {
	Items []ExternalTrainingResponse `json:"items"`
}

type DeleteExternalTrainingResponse struct {
	Message string `json:"message"`
}