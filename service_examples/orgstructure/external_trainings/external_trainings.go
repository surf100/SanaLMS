package external_trainings

import (
	"context"
	"strings"
	"time"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.app/db/ent/externaltrainingevent"
	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

//encore:api auth method=POST path=/external-trainings
var (
	db     = sqldb.Named("lms")
	Client = newEntClient()
)

func newEntClient() *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, db.Stdlib())
	return ent.NewClient(ent.Driver(drv))
}

var requireMinRole = authhandler.RequireMinRole

//encore:api auth method=POST path=/external-trainings
func CreateExternalTraining(ctx context.Context, req *CreateExternalTrainingRequest) (*ExternalTrainingResponse, error) {
	ad, err := requireMinRole(authhandler.RoleADM)
	if err != nil {
		return nil, err
	}

	if req == nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("request body is required").Err()
	}

	if strings.TrimSpace(req.Name) == "" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("name is required").Err()
	}

	if strings.TrimSpace(req.SupplierID) == "" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("supplier_id is required").Err()
	}

	if strings.TrimSpace(req.ContractID) == "" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("contract_id is required").Err()
	}

	if req.Capacity <= 0 {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("capacity must be > 0").Err()
	}

	if req.SupplierCostVAT < 0 {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("supplier_cost_vat must be >= 0").Err()
	}

	if req.StartDate.Before(time.Now()) {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("start_date cannot be in the past").Err()
	}

	supplierID, err := uuid.Parse(req.SupplierID)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid supplier_id").Err()
	}

	contractID, err := uuid.Parse(req.ContractID)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid contract_id").Err()
	}

	builder := Client.ExternalTrainingEvent.
		Create().
		SetName(strings.TrimSpace(req.Name)).
		SetFormat(req.Format).
		SetCapacity(req.Capacity).
		SetSupplierCostVat(req.SupplierCostVAT).
		SetStartDate(req.StartDate).
		SetSupplierID(supplierID).
		SetContractID(contractID)

	if strings.TrimSpace(req.CategoryID) != "" {
		categoryID, err := uuid.Parse(req.CategoryID)
		if err != nil {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid category_id").Err()
		}
		builder.SetCategoryID(categoryID)
	}

	if strings.TrimSpace(req.ResponsibleUserID) != "" {
		responsibleUserID, err := uuid.Parse(req.ResponsibleUserID)
		if err != nil {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid responsible_user_id").Err()
		}
		builder.SetResponsibleUserID(responsibleUserID)
	}

	et, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}

	return toResponse(et, ad), nil
}

//encore:api auth method=GET path=/external-trainings/:id
func GetExternalTraining(ctx context.Context, id string) (*ExternalTrainingResponse, error) {
	ad, err := requireMinRole(authhandler.RoleHR)
	if err != nil {
		return nil, err
	}

	trainingID, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid id").Err()
	}

	et, err := Client.ExternalTrainingEvent.
		Query().
		Where(externaltrainingevent.IDEQ(trainingID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("not found").Err()
		}
		return nil, err
	}

	return toResponse(et, ad), nil
}

//encore:api auth method=GET path=/external-trainings
func ListExternalTrainings(ctx context.Context) (*ListExternalTrainingsResponse, error) {
	ad, err := requireMinRole(authhandler.RoleHR)
	if err != nil {
		return nil, err
	}

	rows, err := Client.ExternalTrainingEvent.
		Query().
		Where(externaltrainingevent.IsDeletedEQ(false)).
		Order(ent.Desc(externaltrainingevent.FieldCreatedAt)).
		All(ctx)

	if err != nil {
		return nil, errs.B().
			Code(errs.Internal).
			Msg("failed to list external trainings").
			Cause(err).
			Err()
	}

	items := make([]ExternalTrainingResponse, 0, len(rows))
	for _, r := range rows {
		items = append(items, *toResponse(r, ad))
	}

	return &ListExternalTrainingsResponse{
		Items: items,
	}, nil
}

//encore:api auth method=PATCH path=/external-trainings/:id
func UpdateExternalTraining(ctx context.Context, id string, req *UpdateExternalTrainingRequest) (*ExternalTrainingResponse, error) {
	ad, err := requireMinRole(authhandler.RoleADM)
	if err != nil {
		return nil, err
	}
	

	trainingID, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid id").Err()
	}

	builder := Client.ExternalTrainingEvent.UpdateOneID(trainingID)

	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("name cannot be empty").Err()
		}
		builder.SetName(strings.TrimSpace(*req.Name))
	}

	if req.Format != nil {
		builder.SetFormat(*req.Format)
	}

	if req.Capacity != nil {
		if *req.Capacity <= 0 {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("capacity must be > 0").Err()
		}
		builder.SetCapacity(*req.Capacity)
	}

	if req.SupplierCostVAT != nil {
		if *req.SupplierCostVAT < 0 {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("supplier_cost_vat must be >= 0").Err()
		}
		builder.SetSupplierCostVat(*req.SupplierCostVAT)
	}

	if req.StartDate != nil {
		if req.StartDate.Before(time.Now()) {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("start_date cannot be in the past").Err()
		}
		builder.SetStartDate(*req.StartDate)
	}

	if req.CategoryID != nil {
		if strings.TrimSpace(*req.CategoryID) == "" {
			builder.ClearCategoryID()
		} else {
			categoryID, err := uuid.Parse(*req.CategoryID)
			if err != nil {
				return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid category_id").Err()
			}
			builder.SetCategoryID(categoryID)
		}
	}

	if req.ResponsibleUserID != nil {
		if strings.TrimSpace(*req.ResponsibleUserID) == "" {
			builder.ClearResponsibleUserID()
		} else {
			responsibleUserID, err := uuid.Parse(*req.ResponsibleUserID)
			if err != nil {
				return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid responsible_user_id").Err()
			}
			builder.SetResponsibleUserID(responsibleUserID)
		}
	}

	et, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("not found").Err()
		}
		return nil, err
	}

	return toResponse(et, ad), nil
}

//encore:api auth method=DELETE path=/external-trainings/:id
func DeleteExternalTraining(ctx context.Context, id string) (*DeleteExternalTrainingResponse, error) {
	// _, err := requireMinRole(authhandler.RoleADM)
	// if err != nil {
	// 	return nil, err
	// }

	trainingID, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg("invalid id").
			Err()
	}

	err = Client.ExternalTrainingEvent.
		UpdateOneID(trainingID).
		SetIsDeleted(true).
		Exec(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().
				Code(errs.NotFound).
				Msg("not found").
				Err()
		}
		return nil, errs.B().
			Code(errs.Internal).
			Msg("failed to delete external training").
			Cause(err).
			Err()
	}

	return &DeleteExternalTrainingResponse{
		Message: "external training deleted",
	}, nil
}

func toResponse(et *ent.ExternalTrainingEvent, ad *authhandler.AuthData) *ExternalTrainingResponse {
	resp := &ExternalTrainingResponse{
		ID:         et.ID.String(),
		Name:       et.Name,
		StartDate:  et.StartDate,
		CreatedAt:  et.CreatedAt,
		SupplierID: et.SupplierID.String(),
		ContractID: et.ContractID.String(),
		IsDeleted:  et.IsDeleted,
	}

	if et.Format != nil {
		resp.Format = *et.Format
	}

	if et.Capacity != nil {
		resp.Capacity = *et.Capacity
	}

	if et.CategoryID != nil {
		resp.CategoryID = et.CategoryID.String()
	}

	if et.ResponsibleUserID != nil {
		resp.ResponsibleUserID = et.ResponsibleUserID.String()
	}

	// HR не видит supplier_cost_vat
	if ad.Role != authhandler.RoleHR {
		if et.SupplierCostVat != nil {
			v := *et.SupplierCostVat
			resp.SupplierCostVAT = &v
		}
	}

	return resp
}