package categories

import (
	"context"
	"strings"
	"testing"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/google/uuid"

	"encore.app/auth/authhandler"
)

// ════ HELPERS ════

func ctx() context.Context {
	return auth.WithContext(
		context.Background(),
		auth.UID("test-user"),
		&authhandler.AuthData{
			Role:      authhandler.RoleADM,
			CompanyID: "00000000-0000-0000-0000-000000000001",
		},
	)
}

func strPtr(s string) *string { return &s }

func uniqueName(prefix string) string {
	return prefix + "-" + uuid.NewString()
}

func makeCategory(t *testing.T, name string) *Category {
	t.Helper()

	resp, err := CreateCategory(ctx(), &CreateCategoryRequest{Name: name})
	if err != nil {
		t.Fatalf("makeCategory: %v", err)
	}

	return &resp.Category
}

// ════ CREATE ════

func TestCreateCategory_Success(t *testing.T) {
	name := uniqueName("cat")

	resp, err := CreateCategory(ctx(), &CreateCategoryRequest{Name: name})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Category.ID == "" {
		t.Error("expected non-empty ID")
	}

	if resp.Category.Name != name {
		t.Errorf("expected name %q, got %q", name, resp.Category.Name)
	}

	if resp.Category.Description != nil {
		t.Error("expected nil description when not provided")
	}
}

func TestCreateCategory_WithDescription(t *testing.T) {
	name := uniqueName("cat-desc")
	desc := "A useful description"

	resp, err := CreateCategory(ctx(), &CreateCategoryRequest{
		Name:        name,
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Category.Description == nil {
		t.Fatal("expected description to be set")
	}

	if *resp.Category.Description != desc {
		t.Errorf("expected description %q, got %q", desc, *resp.Category.Description)
	}
}

func TestCreateCategory_EmptyName(t *testing.T) {
	_, err := CreateCategory(ctx(), &CreateCategoryRequest{Name: ""})
	if err == nil {
		t.Fatal("expected error for empty name")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateCategory_WhitespaceName(t *testing.T) {
	_, err := CreateCategory(ctx(), &CreateCategoryRequest{Name: "   "})
	if err == nil {
		t.Fatal("expected error for whitespace-only name")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateCategory_NilRequest(t *testing.T) {
	_, err := CreateCategory(ctx(), nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateCategory_NameIsTrimmed(t *testing.T) {
	name := uniqueName("trim")

	resp, err := CreateCategory(
		ctx(),
		&CreateCategoryRequest{Name: "  " + name + "  "},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Category.Name != name {
		t.Errorf("expected trimmed name %q, got %q", name, resp.Category.Name)
	}
}

// ════ GET ════

func TestGetCategory_Success(t *testing.T) {
	cat := makeCategory(t, uniqueName("get"))

	resp, err := GetCategory(ctx(), cat.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Category.ID != cat.ID {
		t.Errorf("expected ID %q, got %q", cat.ID, resp.Category.ID)
	}

	if resp.Category.Name != cat.Name {
		t.Errorf("expected name %q, got %q", cat.Name, resp.Category.Name)
	}
}

func TestGetCategory_NotFound(t *testing.T) {
	_, err := GetCategory(ctx(), uuid.New().String())
	if err == nil {
		t.Fatal("expected error for non-existent ID")
	}

	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestGetCategory_InvalidID(t *testing.T) {
	_, err := GetCategory(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid UUID")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

// ════ LIST ════

func TestListCategories_ReturnsCreated(t *testing.T) {
	name := uniqueName("list")
	cat := makeCategory(t, name)

	resp, err := ListCategories(ctx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false

	for _, c := range resp.Categories {
		if c.ID == cat.ID {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected created category to appear in list")
	}
}

func TestListCategories_OrderedByName(t *testing.T) {
	prefix := "zzz-" + uuid.NewString()

	makeCategory(t, prefix+"-B")
	makeCategory(t, prefix+"-A")

	resp, err := ListCategories(ctx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	idxA, idxB := -1, -1

	for i, c := range resp.Categories {
		if strings.HasSuffix(c.Name, "-A") &&
			strings.HasPrefix(c.Name, prefix) {
			idxA = i
		}

		if strings.HasSuffix(c.Name, "-B") &&
			strings.HasPrefix(c.Name, prefix) {
			idxB = i
		}
	}

	if idxA == -1 || idxB == -1 {
		t.Fatal("could not find both test categories in list")
	}

	if idxA > idxB {
		t.Error("expected A to come before B when ordered by name")
	}
}

// ════ UPDATE ════

func TestUpdateCategory_Name(t *testing.T) {
	cat := makeCategory(t, uniqueName("upd"))
	newName := uniqueName("updated")

	resp, err := UpdateCategory(ctx(), cat.ID, &UpdateCategoryRequest{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Category.Name != newName {
		t.Errorf("expected name %q, got %q", newName, resp.Category.Name)
	}
}

func TestUpdateCategory_Description(t *testing.T) {
	cat := makeCategory(t, uniqueName("upd-desc"))
	desc := "New description"

	resp, err := UpdateCategory(ctx(), cat.ID, &UpdateCategoryRequest{
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Category.Description == nil ||
		*resp.Category.Description != desc {
		t.Errorf("expected description %q", desc)
	}
}

func TestUpdateCategory_EmptyNameRejected(t *testing.T) {
	cat := makeCategory(t, uniqueName("upd-empty"))
	empty := ""

	_, err := UpdateCategory(
		ctx(),
		cat.ID,
		&UpdateCategoryRequest{Name: &empty},
	)
	if err == nil {
		t.Fatal("expected error for empty name update")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateCategory_NotFound(t *testing.T) {
	name := uniqueName("ghost")

	_, err := UpdateCategory(
		ctx(),
		uuid.New().String(),
		&UpdateCategoryRequest{Name: &name},
	)
	if err == nil {
		t.Fatal("expected error for non-existent category")
	}

	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestUpdateCategory_InvalidID(t *testing.T) {
	name := "x"

	_, err := UpdateCategory(
		ctx(),
		"bad-id",
		&UpdateCategoryRequest{Name: &name},
	)
	if err == nil {
		t.Fatal("expected error for invalid UUID")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

// ════ DELETE ════

func TestDeleteCategory_Success(t *testing.T) {
	cat := makeCategory(t, uniqueName("del"))

	resp, err := DeleteCategory(ctx(), cat.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Message == "" {
		t.Error("expected non-empty message")
	}

	_, err = GetCategory(ctx(), cat.ID)
	if err == nil {
		t.Fatal("expected category to be gone after delete")
	}

	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound after delete, got %v", errs.Code(err))
	}
}

func TestDeleteCategory_NotFound(t *testing.T) {
	_, err := DeleteCategory(ctx(), uuid.New().String())
	if err == nil {
		t.Fatal("expected error for non-existent category")
	}

	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestDeleteCategory_InvalidID(t *testing.T) {
	_, err := DeleteCategory(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid UUID")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

// ════ FULL LIFECYCLE ════

func TestCategory_FullLifecycle(t *testing.T) {
	name := uniqueName("lifecycle")
	desc := "Initial description"

	createResp, err := CreateCategory(ctx(), &CreateCategoryRequest{
		Name:        name,
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	id := createResp.Category.ID

	getResp, err := GetCategory(ctx(), id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if getResp.Category.Name != name {
		t.Errorf(
			"expected name %q, got %q",
			name,
			getResp.Category.Name,
		)
	}

	newName := uniqueName("lifecycle-updated")

	_, err = UpdateCategory(
		ctx(),
		id,
		&UpdateCategoryRequest{Name: &newName},
	)
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	getResp2, err := GetCategory(ctx(), id)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}

	if getResp2.Category.Name != newName {
		t.Errorf(
			"expected updated name %q, got %q",
			newName,
			getResp2.Category.Name,
		)
	}

	_, err = DeleteCategory(ctx(), id)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = GetCategory(ctx(), id)

	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound after delete, got %v", errs.Code(err))
	}
}