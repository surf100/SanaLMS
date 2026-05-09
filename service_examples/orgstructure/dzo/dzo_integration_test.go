package dzo

import (
	"context"
	"testing"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/google/uuid"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
)

// ════ HELPERS ════

func newClientID(t *testing.T) string {
	t.Helper()
	id := uuid.New()
	_, err := Client.Company.
		Create().
		SetID(id).
		SetName("company-" + id.String()).
		Save(context.Background())
	if err != nil && !ent.IsConstraintError(err) {
		t.Fatalf("newClientID: %v", err)
	}
	return id.String()
}

func saCtx(companyID string) context.Context {
	return auth.WithContext(
		context.Background(),
		auth.UID("sa-user"),
		&authhandler.AuthData{
			Role:      authhandler.RoleSA,
			CompanyID: companyID,
		},
	)
}

func admCtxFor(companyID string) context.Context {
	return auth.WithContext(
		context.Background(),
		auth.UID("adm-user"),
		&authhandler.AuthData{
			Role:      authhandler.RoleADM,
			CompanyID: companyID,
		},
	)
}

func hrCtxFor(dzoID string) context.Context {
	return auth.WithContext(
		context.Background(),
		auth.UID("hr-user"),
		&authhandler.AuthData{
			Role:  authhandler.RoleHR,
			DzoID: dzoID,
		},
	)
}

func empCtx() context.Context {
	return auth.WithContext(
		context.Background(),
		auth.UID("emp-user"),
		&authhandler.AuthData{Role: authhandler.RoleEMP},
	)
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

func uniqueName(prefix string) string {
	return prefix + "-" + uuid.NewString()
}

func makeDZOFor(t *testing.T, clientID string) *DZO {
	t.Helper()

	resp, err := CreateDZO(
		admCtxFor(clientID),
		&CreateDZORequest{
			ClientID: clientID,
			Name:     uniqueName("dzo"),
		},
	)
	if err != nil {
		t.Fatalf("makeDZOFor: %v", err)
	}

	return &resp.DZO
}

func insertEmployeeForDZO(t *testing.T, dzoID uuid.UUID) {
	t.Helper()

	dzoEntity, err := Client.DzoOrganization.
		Get(context.Background(), dzoID)
	if err != nil {
		t.Fatalf("fetch dzo: %v", err)
	}

	_, err = Client.Employee.
		Create().
		SetClientID(dzoEntity.ClientID).
		SetDzoID(dzoID).
		SetFullName("Test Employee").
		SetEmail("test@example.com").
		Save(context.Background())

	if err != nil {
		t.Fatalf("insertEmployeeForDZO: %v", err)
	}
}

// ════ CREATE ════

func TestCreateDZO_WithOptionalFields(t *testing.T) {
	cid := newClientID(t)
	short := "TDZ"
	bin := "123456789012"

	resp, err := CreateDZO(admCtxFor(cid), &CreateDZORequest{
		ClientID:  cid,
		Name:      uniqueName("full"),
		ShortName: &short,
		BIN:       &bin,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.DZO.ShortName == nil || *resp.DZO.ShortName != short {
		t.Errorf("expected short_name %q", short)
	}

	if resp.DZO.BIN == nil || *resp.DZO.BIN != bin {
		t.Errorf("expected bin %q", bin)
	}
}

func TestCreateDZO_DuplicateNameRejected(t *testing.T) {
	cid := newClientID(t)
	name := uniqueName("dup")

	_, err := CreateDZO(admCtxFor(cid), &CreateDZORequest{
		ClientID: cid,
		Name:     name,
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = CreateDZO(admCtxFor(cid), &CreateDZORequest{
		ClientID: cid,
		Name:     name,
	})

	if err == nil {
		t.Fatal("expected error")
	}

	if errs.Code(err) != errs.AlreadyExists {
		t.Errorf("expected AlreadyExists, got %v", errs.Code(err))
	}
}

func TestCreateDZO_InvalidClientID(t *testing.T) {
	ctx := auth.WithContext(
		context.Background(),
		auth.UID("sa-user"),
		&authhandler.AuthData{
			Role: authhandler.RoleSA,
		},
	)

	_, err := CreateDZO(ctx, &CreateDZORequest{
		ClientID: "not-a-uuid",
		Name:     uniqueName("bad"),
	})

	if err == nil {
		t.Fatal("expected error for invalid client_id")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateDZO_ClientNotFound(t *testing.T) {
	randomID := uuid.New().String()

	ctx := auth.WithContext(
		context.Background(),
		auth.UID("sa"),
		&authhandler.AuthData{
			Role: authhandler.RoleSA,
		},
	)

	_, err := CreateDZO(ctx, &CreateDZORequest{
		ClientID: randomID,
		Name:     uniqueName("missing"),
	})

	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestCreateDZO_EMPDenied(t *testing.T) {
	_, err := CreateDZO(empCtx(), &CreateDZORequest{
		ClientID: uuid.NewString(),
		Name:     uniqueName("emp"),
	})

	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf("expected PermissionDenied")
	}
}

// ════ GET ════

func TestGetDZO_InvalidID(t *testing.T) {
	cid := newClientID(t)

	_, err := GetDZO(admCtxFor(cid), "bad-id")

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument")
	}
}

func TestGetDZO_NotFound(t *testing.T) {
	cid := newClientID(t)

	_, err := GetDZO(admCtxFor(cid), uuid.NewString())

	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound")
	}
}

// ════ LIST ════

func TestListDZO_SA_SeesAll(t *testing.T) {
	c1 := newClientID(t)
	c2 := newClientID(t)

	d1 := makeDZOFor(t, c1)
	d2 := makeDZOFor(t, c2)

	resp, err := ListDZO(saCtx(c1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found1 := false
	found2 := false

	for _, d := range resp.DZOs {
		if d.ID == d1.ID {
			found1 = true
		}
		if d.ID == d2.ID {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Error("SA should see all")
	}
}

func TestListDZO_EMPDenied(t *testing.T) {
	_, err := ListDZO(empCtx())

	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf("expected PermissionDenied")
	}
}

// ════ UPDATE ════

func TestUpdateDZO_Name(t *testing.T) {
	cid := newClientID(t)
	dzo := makeDZOFor(t, cid)

	newName := uniqueName("updated")

	resp, err := UpdateDZO(admCtxFor(cid), dzo.ID,
		&UpdateDZORequest{
			Name: &newName,
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.DZO.Name != newName {
		t.Errorf("wrong updated name")
	}
}

func TestUpdateDZO_EmptyNameRejected(t *testing.T) {
	cid := newClientID(t)
	dzo := makeDZOFor(t, cid)

	empty := ""

	_, err := UpdateDZO(admCtxFor(cid), dzo.ID,
		&UpdateDZORequest{
			Name: &empty,
		},
	)

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument")
	}
}

// ════ DELETE ════

func TestDeleteDZO_BlockedWhenHasEmployees(t *testing.T) {
	cid := newClientID(t)
	dzo := makeDZOFor(t, cid)

	id, _ := uuid.Parse(dzo.ID)
	insertEmployeeForDZO(t, id)

	_, err := DeleteDZO(admCtxFor(cid), dzo.ID)

	if errs.Code(err) != errs.FailedPrecondition {
		t.Errorf("expected FailedPrecondition")
	}
}

func TestDeleteDZO_SoftDelete_DisappearsFromList(t *testing.T) {
	cid := newClientID(t)
	dzo := makeDZOFor(t, cid)

	_, err := DeleteDZO(admCtxFor(cid), dzo.ID)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	resp, err := ListDZO(admCtxFor(cid))
	if err != nil {
		t.Fatalf("list failed")
	}

	for _, d := range resp.DZOs {
		if d.ID == dzo.ID {
			t.Error("deleted DZO still visible")
		}
	}
}

// ════ FULL LIFECYCLE ════

func TestDZO_FullLifecycle(t *testing.T) {
	cid := newClientID(t)

	short := "LCT"
	bin := "987654321098"

	createResp, err := CreateDZO(admCtxFor(cid), &CreateDZORequest{
		ClientID:  cid,
		Name:      uniqueName("life"),
		ShortName: &short,
		BIN:       &bin,
	})

	if err != nil {
		t.Fatalf("create: %v", err)
	}

	id := createResp.DZO.ID

	_, err = GetDZO(admCtxFor(cid), id)
	if err != nil {
		t.Fatalf("get failed")
	}

	newName := uniqueName("renamed")

	_, err = UpdateDZO(admCtxFor(cid), id,
		&UpdateDZORequest{
			Name: &newName,
		},
	)

	if err != nil {
		t.Fatalf("update failed")
	}

	_, err = DeleteDZO(admCtxFor(cid), id)
	if err != nil {
		t.Fatalf("delete failed")
	}

	resp, err := ListDZO(admCtxFor(cid))
	if err != nil {
		t.Fatalf("list failed")
	}

	for _, d := range resp.DZOs {
		if d.ID == id {
			t.Error("deleted DZO found in list")
		}
	}
}
