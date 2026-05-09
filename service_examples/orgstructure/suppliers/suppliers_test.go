package suppliers

import (
	"context"
	"testing"

	"encore.app/auth/authhandler"
	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
)

// ════ HELPERS ════

func ctx() context.Context {
	return context.Background()
}

func authCtx(companyID string) context.Context {
	return auth.WithContext(
		context.Background(),
		auth.UID("test-user-id"),
		&authhandler.AuthData{
			KeycloakUserID: "d4f2d180-514d-404d-af1f-12e768227d9b",
			CompanyID:      companyID,
			Email:          "test@test.com",
			Role:           authhandler.RoleSA,
			DzoID:          "1",
		},
	)
}

func strPtr(s string) *string   { return &s }
func f64Ptr(f float64) *float64 { return &f }

func testCompanyID() string {
	return "00000000-0000-0000-0000-000000000001"
}

func makeSupplier(t *testing.T, name string, supplierType SupplierType) *Supplier {
	t.Helper()
	resp, err := CreateSupplier(authCtx(testCompanyID()), &CreateSupplierRequest{
		Type: supplierType,
		Name: name,
	})
	if err != nil {
		t.Fatalf("makeSupplier: %v", err)
	}
	return &resp.Supplier
}

func makeSupplierFull(t *testing.T, name string, supplierType SupplierType, binOrIIN *string, localContentPct *float64) *Supplier {
	t.Helper()
	resp, err := CreateSupplier(authCtx(testCompanyID()), &CreateSupplierRequest{
		Type:            supplierType,
		Name:            name,
		BinOrIIN:        binOrIIN,
		LocalContentPct: localContentPct,
	})
	if err != nil {
		t.Fatalf("makeSupplierFull: %v", err)
	}
	return &resp.Supplier
}

// ════ CREATE ════

func TestCreateSupplier_Success(t *testing.T) {
	resp, err := CreateSupplier(authCtx(testCompanyID()), &CreateSupplierRequest{
		Type: SupplierTypeLegal,
		Name: "Acme LLC",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Supplier.ID == "" {
		t.Error("expected non-empty ID")
	}
	if resp.Supplier.Name != "Acme LLC" {
		t.Errorf("expected name 'Acme LLC', got %q", resp.Supplier.Name)
	}
	if resp.Supplier.Type != SupplierTypeLegal {
		t.Errorf("expected type LEGAL, got %q", resp.Supplier.Type)
	}
	if !resp.Supplier.IsActive {
		t.Error("expected is_active to be true")
	}
}

func TestCreateSupplier_SuccessWithOptionalFields(t *testing.T) {
	bin := "123456789012"
	pct := 75.5

	resp, err := CreateSupplier(authCtx(testCompanyID()), &CreateSupplierRequest{
		Type:            SupplierTypeIndividual,
		Name:            "Solo Trader",
		BinOrIIN:        &bin,
		LocalContentPct: &pct,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Supplier.BinOrIIN == nil || *resp.Supplier.BinOrIIN != bin {
		t.Errorf("expected bin_or_iin %q, got %v", bin, resp.Supplier.BinOrIIN)
	}
	if resp.Supplier.LocalContentPct == nil || *resp.Supplier.LocalContentPct != pct {
		t.Errorf("expected local_content_pct %v, got %v", pct, resp.Supplier.LocalContentPct)
	}
}

func TestCreateSupplier_EmptyName(t *testing.T) {
	_, err := CreateSupplier(authCtx(testCompanyID()), &CreateSupplierRequest{
		Type: SupplierTypeLegal,
		Name: "",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateSupplier_WhitespaceName(t *testing.T) {
	_, err := CreateSupplier(authCtx(testCompanyID()), &CreateSupplierRequest{
		Type: SupplierTypeLegal,
		Name: "   ",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateSupplier_InvalidType(t *testing.T) {
	_, err := CreateSupplier(authCtx(testCompanyID()), &CreateSupplierRequest{
		Type: SupplierType("UNKNOWN"),
		Name: "Bad Type Supplier",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

// ════ GET ════

func TestGetSupplier_Success(t *testing.T) {
	s := makeSupplier(t, "Get Me Supplier", SupplierTypeLegal)

	resp, err := GetSupplier(ctx(), s.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Supplier.ID != s.ID {
		t.Errorf("expected ID %q, got %q", s.ID, resp.Supplier.ID)
	}
	if resp.Supplier.Name != s.Name {
		t.Errorf("expected name %q, got %q", s.Name, resp.Supplier.Name)
	}
}

func TestGetSupplier_NotFound(t *testing.T) {
	_, err := GetSupplier(ctx(), "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestGetSupplier_InvalidID(t *testing.T) {
	_, err := GetSupplier(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestGetSupplier_DeletedNotFound(t *testing.T) {
	s := makeSupplier(t, "Soon Gone Supplier", SupplierTypeLegal)

	if _, err := DeleteSupplier(ctx(), s.ID); err != nil {
		t.Fatalf("failed to delete supplier: %v", err)
	}

	_, err := GetSupplier(ctx(), s.ID)
	if err == nil {
		t.Fatal("expected error for deleted supplier, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

// ════ LIST ════

func TestListSuppliers_ReturnsActiveByDefault(t *testing.T) {
	active := makeSupplier(t, "Active Supplier List", SupplierTypeLegal)
	toDelete := makeSupplier(t, "Deleted Supplier List", SupplierTypeIndividual)

	if _, err := DeleteSupplier(ctx(), toDelete.ID); err != nil {
		t.Fatalf("failed to delete supplier: %v", err)
	}

	resp, err := ListSuppliers(ctx(), &ListSuppliersParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundActive := false
	for _, s := range resp.Suppliers {
		if s.ID == toDelete.ID {
			t.Error("deleted supplier should not appear in default list")
		}
		if s.ID == active.ID {
			foundActive = true
		}
	}
	if !foundActive {
		t.Error("active supplier should appear in default list")
	}
}

func TestListSuppliers_ReturnsEmptySliceNotNil(t *testing.T) {
	resp, err := ListSuppliers(ctx(), &ListSuppliersParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Suppliers == nil {
		t.Error("expected []Supplier{}, got nil")
	}
}

func TestListSuppliers_FilterByType(t *testing.T) {
	makeSupplier(t, "Legal Filter Supplier", SupplierTypeLegal)
	makeSupplier(t, "Individual Filter Supplier", SupplierTypeIndividual)

	resp, err := ListSuppliers(ctx(), &ListSuppliersParams{Type: "LEGAL"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range resp.Suppliers {
		if s.Type != SupplierTypeLegal {
			t.Errorf("expected all suppliers to be LEGAL, got %q", s.Type)
		}
	}
}

func TestListSuppliers_FilterByInvalidType(t *testing.T) {
	_, err := ListSuppliers(ctx(), &ListSuppliersParams{Type: "INVALID"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestListSuppliers_FilterByIsActiveFalse(t *testing.T) {
	s := makeSupplier(t, "Inactive Filter Supplier", SupplierTypeLegal)

	if _, err := DeleteSupplier(ctx(), s.ID); err != nil {
		t.Fatalf("failed to delete supplier: %v", err)
	}

	resp, err := ListSuppliers(ctx(), &ListSuppliersParams{IsActive: "false"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, sup := range resp.Suppliers {
		if sup.IsActive {
			t.Error("expected all returned suppliers to be inactive")
		}
	}

	found := false
	for _, sup := range resp.Suppliers {
		if sup.ID == s.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected soft-deleted supplier to appear in is_active=false list")
	}
}

func TestListSuppliers_FilterByIsActiveInvalidValue(t *testing.T) {
	_, err := ListSuppliers(ctx(), &ListSuppliersParams{IsActive: "yes"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestListSuppliers_SearchByName(t *testing.T) {
	makeSupplier(t, "Searchable Unique Zeta Corp", SupplierTypeLegal)
	makeSupplier(t, "Unrelated Alpha Ltd", SupplierTypeIndividual)

	resp, err := ListSuppliers(ctx(), &ListSuppliersParams{Search: "Zeta"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range resp.Suppliers {
		if s.Name != "Searchable Unique Zeta Corp" {
			t.Errorf("unexpected supplier in search results: %q", s.Name)
		}
	}
}

func TestListSuppliers_SearchCaseInsensitive(t *testing.T) {
	makeSupplier(t, "CaseCheck Omega Supplier", SupplierTypeLegal)

	resp, err := ListSuppliers(ctx(), &ListSuppliersParams{Search: "omega"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, s := range resp.Suppliers {
		if s.Name == "CaseCheck Omega Supplier" {
			found = true
		}
	}
	if !found {
		t.Error("expected case-insensitive search to find 'CaseCheck Omega Supplier'")
	}
}

func TestListSuppliers_SearchNoResults(t *testing.T) {
	resp, err := ListSuppliers(ctx(), &ListSuppliersParams{Search: "xyzzy_nonexistent_12345"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Suppliers) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Suppliers))
	}
}

func TestListSuppliers_CombinedFilters(t *testing.T) {
	makeSupplier(t, "Combined Legal Supplier", SupplierTypeLegal)
	makeSupplier(t, "Combined Individual Supplier", SupplierTypeIndividual)

	resp, err := ListSuppliers(ctx(), &ListSuppliersParams{
		Type:   "LEGAL",
		Search: "Combined",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range resp.Suppliers {
		if s.Type != SupplierTypeLegal {
			t.Errorf("expected only LEGAL suppliers, got %q", s.Type)
		}
	}
}

// ════ UPDATE ════

func TestUpdateSupplier_SuccessName(t *testing.T) {
	s := makeSupplier(t, "Before Update Supplier", SupplierTypeLegal)
	newName := "After Update Supplier"

	resp, err := UpdateSupplier(ctx(), s.ID, &UpdateSupplierRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Supplier.Name != newName {
		t.Errorf("expected name %q, got %q", newName, resp.Supplier.Name)
	}
	if resp.Supplier.Type != s.Type {
		t.Errorf("expected type %q to be unchanged, got %q", s.Type, resp.Supplier.Type)
	}
}

func TestUpdateSupplier_SuccessType(t *testing.T) {
	s := makeSupplier(t, "Type Switch Supplier", SupplierTypeLegal)
	newType := SupplierTypeIndividual

	resp, err := UpdateSupplier(ctx(), s.ID, &UpdateSupplierRequest{
		Type: &newType,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Supplier.Type != SupplierTypeIndividual {
		t.Errorf("expected type INDIVIDUAL, got %q", resp.Supplier.Type)
	}
}

func TestUpdateSupplier_SuccessOptionalFields(t *testing.T) {
	s := makeSupplier(t, "Optional Fields Supplier", SupplierTypeLegal)
	bin := "987654321098"
	pct := 50.0

	resp, err := UpdateSupplier(ctx(), s.ID, &UpdateSupplierRequest{
		BinOrIIN:        &bin,
		LocalContentPct: &pct,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Supplier.BinOrIIN == nil || *resp.Supplier.BinOrIIN != bin {
		t.Errorf("expected bin_or_iin %q, got %v", bin, resp.Supplier.BinOrIIN)
	}
	if resp.Supplier.LocalContentPct == nil || *resp.Supplier.LocalContentPct != pct {
		t.Errorf("expected local_content_pct %v, got %v", pct, resp.Supplier.LocalContentPct)
	}
}

func TestUpdateSupplier_EmptyName(t *testing.T) {
	s := makeSupplier(t, "Empty Name Update Supplier", SupplierTypeLegal)
	empty := ""

	_, err := UpdateSupplier(ctx(), s.ID, &UpdateSupplierRequest{Name: &empty})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateSupplier_InvalidType(t *testing.T) {
	s := makeSupplier(t, "Invalid Type Update Supplier", SupplierTypeLegal)
	bad := SupplierType("UNKNOWN")

	_, err := UpdateSupplier(ctx(), s.ID, &UpdateSupplierRequest{Type: &bad})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateSupplier_NotFound(t *testing.T) {
	newName := "Ghost"
	_, err := UpdateSupplier(ctx(), "00000000-0000-0000-0000-000000000000", &UpdateSupplierRequest{Name: &newName})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestUpdateSupplier_InvalidID(t *testing.T) {
	newName := "Bad ID"
	_, err := UpdateSupplier(ctx(), "not-a-uuid", &UpdateSupplierRequest{Name: &newName})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

// ════ DELETE ════

func TestDeleteSupplier_SuccessSoftDeletes(t *testing.T) {
	s := makeSupplier(t, "To Soft Delete Supplier", SupplierTypeLegal)

	resp, err := DeleteSupplier(ctx(), s.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}

	// Should not appear in default active list
	listResp, err := ListSuppliers(ctx(), &ListSuppliersParams{})
	if err != nil {
		t.Fatalf("unexpected error listing: %v", err)
	}
	for _, sup := range listResp.Suppliers {
		if sup.ID == s.ID {
			t.Error("soft-deleted supplier should not appear in active list")
		}
	}

	// Should appear in inactive list
	inactiveResp, err := ListSuppliers(ctx(), &ListSuppliersParams{IsActive: "false"})
	if err != nil {
		t.Fatalf("unexpected error listing inactive: %v", err)
	}
	found := false
	for _, sup := range inactiveResp.Suppliers {
		if sup.ID == s.ID {
			found = true
			if sup.IsActive {
				t.Error("supplier should be inactive after soft delete")
			}
		}
	}
	if !found {
		t.Error("soft-deleted supplier should appear in is_active=false list")
	}
}

func TestDeleteSupplier_NotFound(t *testing.T) {
	_, err := DeleteSupplier(ctx(), "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestDeleteSupplier_DoubleDelete(t *testing.T) {
	s := makeSupplier(t, "Double Delete Supplier", SupplierTypeLegal)

	if _, err := DeleteSupplier(ctx(), s.ID); err != nil {
		t.Fatalf("first delete failed: %v", err)
	}

	_, err := DeleteSupplier(ctx(), s.ID)
	if err == nil {
		t.Fatal("expected error on second delete, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound on second delete, got %v", errs.Code(err))
	}
}

func TestDeleteSupplier_InvalidID(t *testing.T) {
	_, err := DeleteSupplier(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}
