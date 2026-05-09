package external_trainings

import (
	"context"
	"testing"
	"time"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.dev/beta/errs"
	"github.com/google/uuid"
)

func ctx() context.Context {
	return context.Background()
}

func stubRequireMinRole(role authhandler.UserRole) func() {
	prev := requireMinRole
	requireMinRole = func(minRole authhandler.UserRole) (*authhandler.AuthData, error) {
		ad := &authhandler.AuthData{
			KeycloakUserID: "test-user",
			Email:          "test@example.com",
			Role:           role,
			CompanyID:      "11111111-1111-1111-1111-111111111111",
			DzoID:          "22222222-2222-2222-2222-222222222222",
		}
		if ad.Role.Priority() < minRole.Priority() {
			return nil, errs.B().
				Code(errs.PermissionDenied).
				Msg("Недостаточно прав").
				Err()
		}
		return ad, nil
	}
	return func() { requireMinRole = prev }
}

func validCreateReq() *CreateExternalTrainingRequest {
	return &CreateExternalTrainingRequest{
		Name:            "External Safety Training",
		CategoryID:      uuid.NewString(),
		Format:          "OFFLINE",
		Capacity:        25,
		SupplierID:      uuid.NewString(),
		ContractID:      uuid.NewString(),
		SupplierCostVAT: 150000,
		StartDate:       time.Now().Add(24 * time.Hour),
	}
}

func strPtr(v string) *string { return &v }
func intPtr(v int) *int       { return &v }
func floatPtr(v float64) *float64 {
	return &v
}
func timePtr(v time.Time) *time.Time { return &v }

func TestToResponse_AdminSeesSupplierCostVAT(t *testing.T) {
	cost := 12345.0
	format := "ONLINE"
	capacity := 30
	categoryID := uuid.New()
	responsibleID := uuid.New()

	et := &ent.ExternalTrainingEvent{
		ID:                uuid.New(),
		Name:              "Training A",
		Format:            &format,
		Capacity:          &capacity,
		SupplierCostVat:   &cost,
		StartDate:         time.Now().Add(24 * time.Hour),
		CreatedAt:         time.Now(),
		CategoryID:        &categoryID,
		SupplierID:        uuid.New(),
		ContractID:        uuid.New(),
		ResponsibleUserID: &responsibleID,
	}

	ad := &authhandler.AuthData{Role: authhandler.RoleADM}

	resp := toResponse(et, ad)

	if resp.ID == "" {
		t.Error("expected non-empty ID")
	}
	if resp.Name != et.Name {
		t.Errorf("expected name %q, got %q", et.Name, resp.Name)
	}
	if resp.SupplierCostVAT == nil {
		t.Fatal("expected supplier_cost_vat to be visible for admin")
	}
	if *resp.SupplierCostVAT != cost {
		t.Errorf("expected supplier_cost_vat %v, got %v", cost, *resp.SupplierCostVAT)
	}
	if resp.CategoryID != categoryID.String() {
		t.Errorf("expected category_id %q, got %q", categoryID.String(), resp.CategoryID)
	}
	if resp.ResponsibleUserID != responsibleID.String() {
		t.Errorf("expected responsible_user_id %q, got %q", responsibleID.String(), resp.ResponsibleUserID)
	}
}

func TestToResponse_HRDoesNotSeeSupplierCostVAT(t *testing.T) {
	cost := 12345.0
	format := "ONLINE"
	capacity := 30

	et := &ent.ExternalTrainingEvent{
		ID:              uuid.New(),
		Name:            "Training B",
		Format:          &format,
		Capacity:        &capacity,
		SupplierCostVat: &cost,
		StartDate:       time.Now().Add(24 * time.Hour),
		CreatedAt:       time.Now(),
		SupplierID:      uuid.New(),
		ContractID:      uuid.New(),
	}

	ad := &authhandler.AuthData{Role: authhandler.RoleHR}

	resp := toResponse(et, ad)

	if resp.SupplierCostVAT != nil {
		t.Fatal("expected supplier_cost_vat to be hidden for HR")
	}
}

func TestToResponse_OptionalFieldsEmpty(t *testing.T) {
	et := &ent.ExternalTrainingEvent{
		ID:         uuid.New(),
		Name:       "Training C",
		StartDate:  time.Now().Add(24 * time.Hour),
		CreatedAt:  time.Now(),
		SupplierID: uuid.New(),
		ContractID: uuid.New(),
	}

	ad := &authhandler.AuthData{Role: authhandler.RoleADM}

	resp := toResponse(et, ad)

	if resp.Format != "" {
		t.Errorf("expected empty format, got %q", resp.Format)
	}
	if resp.Capacity != 0 {
		t.Errorf("expected zero capacity, got %d", resp.Capacity)
	}
	if resp.CategoryID != "" {
		t.Errorf("expected empty category_id, got %q", resp.CategoryID)
	}
	if resp.ResponsibleUserID != "" {
		t.Errorf("expected empty responsible_user_id, got %q", resp.ResponsibleUserID)
	}
	if resp.SupplierCostVAT != nil {
		t.Errorf("expected nil supplier_cost_vat, got %v", *resp.SupplierCostVAT)
	}
}

func TestCreateExternalTraining_PermissionDeniedForHR(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleHR)
	defer restore()

	_, err := CreateExternalTraining(ctx(), validCreateReq())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", errs.Code(err))
	}
}

func TestCreateExternalTraining_PermissionDeniedForEMP(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleEMP)
	defer restore()

	_, err := CreateExternalTraining(ctx(), validCreateReq())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", errs.Code(err))
	}
}

func TestCreateExternalTraining_NilRequest(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	_, err := CreateExternalTraining(ctx(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateExternalTraining_EmptyName(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	req := validCreateReq()
	req.Name = ""

	_, err := CreateExternalTraining(ctx(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateExternalTraining_WhitespaceName(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	req := validCreateReq()
	req.Name = "   "

	_, err := CreateExternalTraining(ctx(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateExternalTraining_EmptySupplierID(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	req := validCreateReq()
	req.SupplierID = ""

	_, err := CreateExternalTraining(ctx(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateExternalTraining_EmptyContractID(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	req := validCreateReq()
	req.ContractID = ""

	_, err := CreateExternalTraining(ctx(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateExternalTraining_CapacityMustBePositive(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	cases := []int{0, -1, -10}
	for _, c := range cases {
		req := validCreateReq()
		req.Capacity = c

		_, err := CreateExternalTraining(ctx(), req)
		if err == nil {
			t.Fatalf("capacity=%d: expected error, got nil", c)
		}
		if errs.Code(err) != errs.InvalidArgument {
			t.Fatalf("capacity=%d: expected InvalidArgument, got %v", c, errs.Code(err))
		}
	}
}

func TestCreateExternalTraining_SupplierCostVATCannotBeNegative(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	req := validCreateReq()
	req.SupplierCostVAT = -100

	_, err := CreateExternalTraining(ctx(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateExternalTraining_StartDateCannotBePast(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	req := validCreateReq()
	req.StartDate = time.Now().Add(-2 * time.Hour)

	_, err := CreateExternalTraining(ctx(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateExternalTraining_InvalidSupplierID(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	req := validCreateReq()
	req.SupplierID = "not-a-uuid"

	_, err := CreateExternalTraining(ctx(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateExternalTraining_InvalidContractID(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	req := validCreateReq()
	req.ContractID = "not-a-uuid"

	_, err := CreateExternalTraining(ctx(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateExternalTraining_InvalidCategoryID(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	req := validCreateReq()
	req.CategoryID = "not-a-uuid"

	_, err := CreateExternalTraining(ctx(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateExternalTraining_InvalidResponsibleUserID(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	req := validCreateReq()
	req.ResponsibleUserID = "not-a-uuid"

	_, err := CreateExternalTraining(ctx(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateExternalTraining_PermissionDeniedForHR(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleHR)
	defer restore()

	name := "Updated Name"
	_, err := UpdateExternalTraining(ctx(), uuid.NewString(), &UpdateExternalTrainingRequest{
		Name: &name,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", errs.Code(err))
	}
}

func TestUpdateExternalTraining_InvalidIDFormat(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	name := "Updated Name"
	_, err := UpdateExternalTraining(ctx(), "not-a-uuid", &UpdateExternalTrainingRequest{
		Name: &name,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateExternalTraining_EmptyName(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	name := "   "
	_, err := UpdateExternalTraining(ctx(), uuid.NewString(), &UpdateExternalTrainingRequest{
		Name: &name,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateExternalTraining_InvalidCapacity(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	capacity := 0
	_, err := UpdateExternalTraining(ctx(), uuid.NewString(), &UpdateExternalTrainingRequest{
		Capacity: &capacity,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateExternalTraining_NegativeSupplierCostVAT(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	cost := -1.0
	_, err := UpdateExternalTraining(ctx(), uuid.NewString(), &UpdateExternalTrainingRequest{
		SupplierCostVAT: &cost,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateExternalTraining_PastStartDate(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	past := time.Now().Add(-24 * time.Hour)
	_, err := UpdateExternalTraining(ctx(), uuid.NewString(), &UpdateExternalTrainingRequest{
		StartDate: &past,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateExternalTraining_InvalidCategoryID(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	bad := "not-a-uuid"
	_, err := UpdateExternalTraining(ctx(), uuid.NewString(), &UpdateExternalTrainingRequest{
		CategoryID: &bad,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateExternalTraining_InvalidResponsibleUserID(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleADM)
	defer restore()

	bad := "not-a-uuid"
	_, err := UpdateExternalTraining(ctx(), uuid.NewString(), &UpdateExternalTrainingRequest{
		ResponsibleUserID: &bad,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestGetExternalTraining_PermissionDeniedForEMP(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleEMP)
	defer restore()

	_, err := GetExternalTraining(ctx(), uuid.NewString())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", errs.Code(err))
	}
}

func TestGetExternalTraining_InvalidIDFormat(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleHR)
	defer restore()

	_, err := GetExternalTraining(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestListExternalTrainings_PermissionDeniedForEMP(t *testing.T) {
	restore := stubRequireMinRole(authhandler.RoleEMP)
	defer restore()

	_, err := ListExternalTrainings(ctx())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", errs.Code(err))
	}
}