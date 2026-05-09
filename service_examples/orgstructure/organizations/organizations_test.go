package organizations

import (
	"context"
	"testing"

	"encore.app/auth/authhandler"
	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
)

// ════ HELPERS ════

const testClientID = "11111111-1111-1111-1111-111111111111"

func ctx() context.Context {
	return auth.WithContext(
		context.Background(),
		auth.UID("test-user"),
		&authhandler.AuthData{
			Role:      authhandler.RoleSA,
			CompanyID: testClientID,
		},
	)
}

func makeOrg(t *testing.T, name, code string, orgType OrgType) *Organization {
	t.Helper()
	resp, err := CreateOrg(ctx(), &CreateOrgRequest{
		Name: name,
		Code: code,
		Type: orgType,
	})
	if err != nil {
		t.Fatalf("makeOrg: %v", err)
	}
	return &resp.Organization
}

func makeOrgWithParent(t *testing.T, name, code string, orgType OrgType, parentID string) *Organization {
	t.Helper()
	resp, err := CreateOrg(ctx(), &CreateOrgRequest{
		Name:     name,
		Code:     code,
		Type:     orgType,
		ParentID: &parentID,
	})
	if err != nil {
		t.Fatalf("makeOrgWithParent: %v", err)
	}
	return &resp.Organization
}

// ════ CREATE ════

func TestCreateOrg_Success(t *testing.T) {
	resp, err := CreateOrg(ctx(), &CreateOrgRequest{
		Name: "Acme Corp",
		Code: "ACME",
		Type: OrgTypeCompany,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Organization.ID == "" {
		t.Error("expected non-empty ID")
	}
	if resp.Organization.Name != "Acme Corp" {
		t.Errorf("expected name 'Acme Corp', got %q", resp.Organization.Name)
	}
	if resp.Organization.Code != "ACME" {
		t.Errorf("expected code 'ACME', got %q", resp.Organization.Code)
	}
	if resp.Organization.Type != OrgTypeCompany {
		t.Errorf("expected type %q, got %q", OrgTypeCompany, resp.Organization.Type)
	}
	if !resp.Organization.IsActive {
		t.Error("expected newly created org to be active")
	}
	if resp.Organization.ParentID != nil {
		t.Errorf("expected nil parent_id, got %v", resp.Organization.ParentID)
	}
	if resp.Organization.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
	if resp.Organization.UpdatedAt.IsZero() {
		t.Error("expected non-zero updated_at")
	}
}

func TestCreateOrg_SuccessWithParentID(t *testing.T) {
	parent := makeOrg(t, "Parent Co", "PARENTCO", OrgTypeCompany)

	resp, err := CreateOrg(ctx(), &CreateOrgRequest{
		Name:     "Child Dept",
		Code:     "CHILDDEPT",
		Type:     OrgTypeDepartment,
		ParentID: &parent.ID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Organization.ParentID == nil || *resp.Organization.ParentID != parent.ID {
		t.Errorf("expected parent_id %q, got %v", parent.ID, resp.Organization.ParentID)
	}
}

func TestCreateOrg_AllValidTypes(t *testing.T) {
	cases := []struct {
		orgType OrgType
		code    string
	}{
		{OrgTypeCompany, "TYPECO"},
		{OrgTypeSubsidiary, "TYPESUB"},
		{OrgTypeDepartment, "TYPEDEPT"},
		{OrgTypeDivision, "TYPEDIV"},
	}
	for _, tc := range cases {
		resp, err := CreateOrg(ctx(), &CreateOrgRequest{
			Name: "Type Test",
			Code: tc.code,
			Type: tc.orgType,
		})
		if err != nil {
			t.Errorf("type %q: unexpected error: %v", tc.orgType, err)
			continue
		}
		if resp.Organization.Type != tc.orgType {
			t.Errorf("type %q: expected type to be saved correctly, got %q", tc.orgType, resp.Organization.Type)
		}
	}
}

func TestCreateOrg_EmptyName(t *testing.T) {
	_, err := CreateOrg(ctx(), &CreateOrgRequest{
		Name: "",
		Code: "NONAME",
		Type: OrgTypeCompany,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateOrg_WhitespaceOnlyName(t *testing.T) {
	_, err := CreateOrg(ctx(), &CreateOrgRequest{
		Name: "   ",
		Code: "WSNAME",
		Type: OrgTypeCompany,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateOrg_EmptyCode(t *testing.T) {
	_, err := CreateOrg(ctx(), &CreateOrgRequest{
		Name: "No Code Org",
		Code: "",
		Type: OrgTypeCompany,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateOrg_InvalidType(t *testing.T) {
	_, err := CreateOrg(ctx(), &CreateOrgRequest{
		Name: "Bad Type Org",
		Code: "BADTYPE",
		Type: OrgType("invalid"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateOrg_DuplicateCode(t *testing.T) {
	makeOrg(t, "Original Org", "DUPCODE", OrgTypeCompany)

	_, err := CreateOrg(ctx(), &CreateOrgRequest{
		Name: "Duplicate Org",
		Code: "DUPCODE",
		Type: OrgTypeSubsidiary,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.AlreadyExists {
		t.Errorf("expected AlreadyExists, got %v", errs.Code(err))
	}
}

func TestCreateOrg_InvalidParentIDFormat(t *testing.T) {
	badParentID := "not-a-uuid"
	_, err := CreateOrg(ctx(), &CreateOrgRequest{
		Name:     "Child Org",
		Code:     "BADPARENTFMT",
		Type:     OrgTypeDepartment,
		ParentID: &badParentID,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateOrg_ParentDoesNotExist(t *testing.T) {
	missingParentID := "11111111-1111-1111-1111-111111111111"
	_, err := CreateOrg(ctx(), &CreateOrgRequest{
		Name:     "Orphan Org",
		Code:     "ORPHANORG",
		Type:     OrgTypeDepartment,
		ParentID: &missingParentID,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.AlreadyExists {
		t.Errorf("expected AlreadyExists (FK constraint), got %v", errs.Code(err))
	}
}

// ════ GET ════

func TestGetOrg_Success(t *testing.T) {
	org := makeOrg(t, "Get Me", "GETME", OrgTypeCompany)

	resp, err := GetOrg(ctx(), org.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Organization.ID != org.ID {
		t.Errorf("expected ID %q, got %q", org.ID, resp.Organization.ID)
	}
	if resp.Organization.Name != org.Name {
		t.Errorf("expected name %q, got %q", org.Name, resp.Organization.Name)
	}
	if resp.Organization.Code != org.Code {
		t.Errorf("expected code %q, got %q", org.Code, resp.Organization.Code)
	}
	if resp.Organization.Type != org.Type {
		t.Errorf("expected type %q, got %q", org.Type, resp.Organization.Type)
	}
	if !resp.Organization.IsActive {
		t.Error("expected org to be active")
	}
}

func TestGetOrg_InvalidIDFormat(t *testing.T) {
	_, err := GetOrg(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestGetOrg_NotFound(t *testing.T) {
	_, err := GetOrg(ctx(), "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestGetOrg_SoftDeletedOrgStillAccessible(t *testing.T) {
	org := makeOrg(t, "Soft Deleted Get", "SOFTGET", OrgTypeCompany)

	if _, err := DeleteOrg(ctx(), org.ID); err != nil {
		t.Fatalf("failed to delete org: %v", err)
	}

	resp, err := GetOrg(ctx(), org.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Organization.ID != org.ID {
		t.Errorf("expected ID %q, got %q", org.ID, resp.Organization.ID)
	}
	if resp.Organization.IsActive {
		t.Error("expected soft-deleted org to remain inactive")
	}
}

// ════ LIST ════

func TestListOrgs_ReturnsOnlyActiveOrgs(t *testing.T) {
	active := makeOrg(t, "Active Org", "ACTIVELIST", OrgTypeCompany)
	toDelete := makeOrg(t, "To Delete", "DELETELIST", OrgTypeSubsidiary)

	if _, err := DeleteOrg(ctx(), toDelete.ID); err != nil {
		t.Fatalf("failed to delete org: %v", err)
	}

	resp, err := ListOrgs(ctx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundActive := false
	for _, o := range resp.Organizations {
		if o.ID == toDelete.ID {
			t.Errorf("deleted org should not appear in list")
		}
		if o.ID == active.ID {
			foundActive = true
		}
	}
	if !foundActive {
		t.Error("active org should appear in list")
	}
}

func TestListOrgs_TotalMatchesOrganizationsLength(t *testing.T) {
	makeOrg(t, "Total Check 1", "TOTAL1", OrgTypeCompany)
	makeOrg(t, "Total Check 2", "TOTAL2", OrgTypeSubsidiary)

	resp, err := ListOrgs(ctx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != len(resp.Organizations) {
		t.Errorf("Total=%d does not match len(Organizations)=%d", resp.Total, len(resp.Organizations))
	}
}

func TestListOrgs_ReturnsEmptySliceNotNil(t *testing.T) {
	resp, err := ListOrgs(ctx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Organizations == nil {
		t.Error("expected []Organization{}, got nil")
	}
}

func TestListOrgs_OrderIsStableByCreatedAtAsc(t *testing.T) {
	first := makeOrg(t, "First Listed", "ORDERFIRST", OrgTypeCompany)
	second := makeOrg(t, "Second Listed", "ORDERSECOND", OrgTypeSubsidiary)

	resp, err := ListOrgs(ctx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	idxFirst, idxSecond := -1, -1
	for i, o := range resp.Organizations {
		if o.ID == first.ID {
			idxFirst = i
		}
		if o.ID == second.ID {
			idxSecond = i
		}
	}

	if idxFirst == -1 || idxSecond == -1 {
		t.Fatal("both orgs should appear in list")
	}
	if idxFirst >= idxSecond {
		t.Errorf("expected first org at idx %d before second at idx %d", idxFirst, idxSecond)
	}
}

// ════ UPDATE ════

func TestUpdateOrg_SuccessUpdatesOnlyProvidedFields(t *testing.T) {
	org := makeOrg(t, "Before Update", "BEFOREUPD", OrgTypeCompany)
	newName := "After Update"

	resp, err := UpdateOrg(ctx(), org.ID, &UpdateOrgRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Organization.Name != newName {
		t.Errorf("expected name %q, got %q", newName, resp.Organization.Name)
	}
	if resp.Organization.Code != org.Code {
		t.Errorf("expected code %q to be unchanged, got %q", org.Code, resp.Organization.Code)
	}
	if resp.Organization.Type != org.Type {
		t.Errorf("expected type %q to be unchanged, got %q", org.Type, resp.Organization.Type)
	}
}

func TestUpdateOrg_EmptyRequestChangesNothing(t *testing.T) {
	org := makeOrg(t, "Stable Org", "STABLE", OrgTypeCompany)

	resp, err := UpdateOrg(ctx(), org.ID, &UpdateOrgRequest{})
	if err != nil {
		t.Fatalf("unexpected error on empty update: %v", err)
	}
	if resp.Organization.Name != org.Name {
		t.Errorf("name changed: %q → %q", org.Name, resp.Organization.Name)
	}
	if resp.Organization.Code != org.Code {
		t.Errorf("code changed: %q → %q", org.Code, resp.Organization.Code)
	}
	if resp.Organization.Type != org.Type {
		t.Errorf("type changed: %q → %q", org.Type, resp.Organization.Type)
	}
}

func TestUpdateOrg_PartialUpdateOnlyCode(t *testing.T) {
	org := makeOrg(t, "Code Before", "CODEBEFORE", OrgTypeCompany)
	newCode := "CODEAFTER"

	resp, err := UpdateOrg(ctx(), org.ID, &UpdateOrgRequest{Code: &newCode})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Organization.Code != newCode {
		t.Errorf("expected code %q, got %q", newCode, resp.Organization.Code)
	}
	if resp.Organization.Name != org.Name {
		t.Errorf("expected name %q to be unchanged, got %q", org.Name, resp.Organization.Name)
	}
}

func TestUpdateOrg_PartialUpdateOnlyParentID(t *testing.T) {
	parent := makeOrg(t, "New Parent", "NEWPARENT", OrgTypeCompany)
	org := makeOrg(t, "Child Before", "CHILDBEFORE", OrgTypeDepartment)

	resp, err := UpdateOrg(ctx(), org.ID, &UpdateOrgRequest{ParentID: &parent.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Organization.ParentID == nil || *resp.Organization.ParentID != parent.ID {
		t.Errorf("expected parent_id %q, got %v", parent.ID, resp.Organization.ParentID)
	}
	if resp.Organization.Name != org.Name {
		t.Errorf("expected name %q to be unchanged, got %q", org.Name, resp.Organization.Name)
	}
}

func TestUpdateOrg_UpdatedAtChanges(t *testing.T) {
	org := makeOrg(t, "Timestamp Org", "TSORG", OrgTypeCompany)
	newName := "Timestamp Updated"

	resp, err := UpdateOrg(ctx(), org.ID, &UpdateOrgRequest{Name: &newName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Organization.UpdatedAt.Before(org.CreatedAt) {
		t.Errorf("updated_at %v should not be before created_at %v", resp.Organization.UpdatedAt, org.CreatedAt)
	}
}

func TestUpdateOrg_InvalidType(t *testing.T) {
	org := makeOrg(t, "Update Bad Type", "UPDBADTYPE", OrgTypeCompany)
	badType := OrgType("invalid")

	_, err := UpdateOrg(ctx(), org.ID, &UpdateOrgRequest{Type: &badType})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateOrg_InvalidIDFormat(t *testing.T) {
	newName := "Ghost"
	_, err := UpdateOrg(ctx(), "not-a-uuid", &UpdateOrgRequest{Name: &newName})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateOrg_InvalidParentIDFormat(t *testing.T) {
	org := makeOrg(t, "Bad Parent Update", "BADPARENTUPD", OrgTypeCompany)
	badParentID := "not-a-uuid"

	_, err := UpdateOrg(ctx(), org.ID, &UpdateOrgRequest{ParentID: &badParentID})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateOrg_NotFound(t *testing.T) {
	newName := "Ghost"
	_, err := UpdateOrg(ctx(), "00000000-0000-0000-0000-000000000000", &UpdateOrgRequest{Name: &newName})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestUpdateOrg_DuplicateCode(t *testing.T) {
	makeOrg(t, "First Org", "FIRSTORG", OrgTypeCompany)
	second := makeOrg(t, "Second Org", "SECONDORG", OrgTypeSubsidiary)

	existingCode := "FIRSTORG"
	_, err := UpdateOrg(ctx(), second.ID, &UpdateOrgRequest{Code: &existingCode})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.AlreadyExists {
		t.Errorf("expected AlreadyExists, got %v", errs.Code(err))
	}
}

func TestUpdateOrg_SoftDeletedOrgStillUpdates(t *testing.T) {
	org := makeOrg(t, "Soft Deleted Update", "SOFTUPD", OrgTypeCompany)
	newName := "Soft Deleted Updated"

	if _, err := DeleteOrg(ctx(), org.ID); err != nil {
		t.Fatalf("failed to delete org: %v", err)
	}

	resp, err := UpdateOrg(ctx(), org.ID, &UpdateOrgRequest{Name: &newName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Organization.Name != newName {
		t.Errorf("expected name %q, got %q", newName, resp.Organization.Name)
	}
	if resp.Organization.IsActive {
		t.Error("expected soft-deleted org to remain inactive after update")
	}
}

// ════ DELETE ════

func TestDeleteOrg_SuccessSoftDeletes(t *testing.T) {
	org := makeOrg(t, "To Soft Delete", "SOFTDEL", OrgTypeCompany)

	resp, err := DeleteOrg(ctx(), org.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}


	listResp, err := ListOrgs(ctx())
	if err != nil {
		t.Fatalf("unexpected error listing: %v", err)
	}
	for _, o := range listResp.Organizations {
		if o.ID == org.ID {
			t.Error("soft-deleted org should not appear in list")
		}
	}

	getResp, err := GetOrg(ctx(), org.ID)
	if err != nil {
		t.Fatalf("GetOrg after soft delete should succeed: %v", err)
	}
	if getResp.Organization.IsActive {
		t.Error("org should be inactive after soft delete")
	}
}

func TestDeleteOrg_InvalidIDFormat(t *testing.T) {
	_, err := DeleteOrg(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestDeleteOrg_NotFound(t *testing.T) {
	_, err := DeleteOrg(ctx(), "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestDeleteOrg_DoubleDelete(t *testing.T) {
	org := makeOrg(t, "Double Delete", "DOUBLEDEL", OrgTypeCompany)

	if _, err := DeleteOrg(ctx(), org.ID); err != nil {
		t.Fatalf("first delete failed: %v", err)
	}

	_, err := DeleteOrg(ctx(), org.ID)
	if err == nil {
		t.Fatal("expected error on second delete, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound on second delete, got %v", errs.Code(err))
	}
}

func TestDeleteOrg_ChildOrgSurvivesParentDelete(t *testing.T) {
	parent := makeOrg(t, "Parent To Delete", "PARENTDEL", OrgTypeCompany)
	child := makeOrgWithParent(t, "Child Survives", "CHILDSURVIVE", OrgTypeDepartment, parent.ID)

	if _, err := DeleteOrg(ctx(), parent.ID); err != nil {
		t.Fatalf("failed to delete parent: %v", err)
	}

	getResp, err := GetOrg(ctx(), child.ID)
	if err != nil {
		t.Fatalf("child org should still exist after parent deleted: %v", err)
	}
	if !getResp.Organization.IsActive {
		t.Error("child org should still be active after parent soft-delete")
	}

	if getResp.Organization.ParentID == nil {
		t.Log("INFO: parent_id was set to null after parent soft-delete (unexpected for soft-delete)")
	}
}