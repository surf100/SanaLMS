package dzo

import (
	"context"
	"testing"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/google/uuid"
)

// testClientID is the client UUID shared across DZO tests.
// The ADM's CompanyID must match this value so the client-scope checks pass.
const testClientID = "11111111-1111-1111-1111-111111111111"

func ctx() context.Context {
	return auth.WithContext(
		context.Background(),
		auth.UID("test-user"),
		&authhandler.AuthData{
			Role:      authhandler.RoleADM,
			CompanyID: testClientID,
		},
	)
}
func makeClient(t *testing.T) uuid.UUID {
	t.Helper()

	id := uuid.MustParse(testClientID)

	_, err := Client.Company.
		Create().
		SetID(id).
		SetName("Test Company").
		Save(ctx())

	if err != nil && !ent.IsConstraintError(err) {
		t.Fatalf("makeClient: %v", err)
	}

	return id
}


func makeDZO(t *testing.T, name string) *DZO {
	id := makeClient(t)
	t.Helper()

	resp, err := CreateDZO(ctx(), &CreateDZORequest{
		ClientID: id.String(),
		Name:     name,
	})
	if err != nil {
		t.Fatalf("makeDZO: %v", err)
	}

	return &resp.DZO
}

// ════ CREATE ════

func TestCreateDZO_Success(t *testing.T) {
	id := makeClient(t)
	resp, err := CreateDZO(ctx(), &CreateDZORequest{
		ClientID: id.String(),
		Name:     "Test",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.DZO.ID == "" {
		t.Error("expected ID")
	}
}

func TestCreateDZO_EmptyName(t *testing.T) {
	id := makeClient(t)
	_, err := CreateDZO(ctx(), &CreateDZORequest{
		ClientID: id.String(),
		Name:     "",
	})

	if err == nil {
		t.Fatal("expected error")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument")
	}
}

func TestCreateDZO_ADM_CannotCreateInOtherClient(t *testing.T) {
	makeClient(t)

	_, err := CreateDZO(ctx(), &CreateDZORequest{
		ClientID: "22222222-2222-2222-2222-222222222222",
		Name:     "Foreign DZO",
	})

	if err == nil {
		t.Fatal("expected error when ADM creates DZO in another client")
	}
	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", errs.Code(err))
	}
}

// ════ GET ════

func TestGetDZO_Success(t *testing.T) {
	dzo := makeDZO(t, "GET")

	resp, err := GetDZO(ctx(), dzo.ID)
	if err != nil {
		t.Fatalf("unexpected error")
	}

	if resp.DZO.ID != dzo.ID {
		t.Error("wrong id")
	}
}

// ════ DELETE ════

func TestDeleteDZO_Success(t *testing.T) {
	dzo := makeDZO(t, "DEL")

	_, err := DeleteDZO(ctx(), dzo.ID)
	if err != nil {
		t.Fatalf("unexpected error")
	}
}

func TestDeleteDZO_NotFound(t *testing.T) {
	_, err := DeleteDZO(ctx(), "00000000-0000-0000-0000-000000000000")

	if err == nil {
		t.Fatal("expected error")
	}

	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound")
	}
}
