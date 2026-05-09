package contractssuppliers

import (
	"context"
	"testing"
	"time"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/google/uuid"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
)

// ════ HELPERS ════

const testCompanyID = "00000000-0000-0000-0000-000000000001"

func admCtx() context.Context {
	return auth.WithContext(
		context.Background(),
		auth.UID("adm-user"),
		&authhandler.AuthData{
			Role:      authhandler.RoleADM,
			CompanyID: testCompanyID,
		},
	)
}

func empCtx() context.Context {
	return auth.WithContext(
		context.Background(),
		auth.UID("emp-user"),
		&authhandler.AuthData{
			Role: authhandler.RoleEMP,
		},
	)
}

func strPtr(s string) *string {
	return &s
}

func f64Ptr(f float64) *float64 {
	return &f
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func uniqueContractNumber() string {
	return "№" + uuid.NewString()[:8]
}

func insertSupplier(t *testing.T) string {
	t.Helper()

	cid := uuid.MustParse(testCompanyID)

	_, err := Client.Company.Create().
		SetID(cid).
		SetName("Test Company").
		Save(context.Background())

	if err != nil && !ent.IsConstraintError(err) {
		t.Fatalf("insertSupplier/company: %v", err)
	}

	sid := uuid.New()

	_, err = Client.Supplier.Create().
		SetID(sid).
		SetClientID(cid).
		SetName("Supplier-" + sid.String()).
		SetType("LEGAL").
		Save(context.Background())

	if err != nil {
		t.Fatalf("insertSupplier: %v", err)
	}

	return sid.String()
}

func makeContract(
	t *testing.T,
	supplierID string,
	amount float64,
) *ContractSupplier {
	t.Helper()

	signedDate := time.Now().Truncate(24 * time.Hour)

	resp, err := CreateContract(
		admCtx(),
		supplierID,
		&CreateContractRequest{
			ContractNumber: uniqueContractNumber(),
			VatFlag:        12,
			SignedDate:     signedDate,
			Amount:         amount,
		},
	)

	if err != nil {
		t.Fatalf("makeContract: %v", err)
	}

	return &resp.Contract
}

// ════ CREATE ════

func TestCreateContract_Success(t *testing.T) {
	sid := insertSupplier(t)

	signedDate := time.Now().Truncate(24 * time.Hour)
	endDate := signedDate.Add(365 * 24 * time.Hour)

	resp, err := CreateContract(
		admCtx(),
		sid,
		&CreateContractRequest{
			ContractNumber: uniqueContractNumber(),
			VatFlag:        12,
			SignedDate:     signedDate,
			EndDate:        &endDate,
			Amount:         500000,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c := resp.Contract

	if c.ID == "" {
		t.Error("expected non-empty ID")
	}

	if c.SupplierID != sid {
		t.Errorf("expected supplier_id %q got %q", sid, c.SupplierID)
	}

	if c.Amount != 500000 {
		t.Errorf("expected amount 500000 got %v", c.Amount)
	}

	if c.TotalWithAmendment != c.Amount {
		t.Error("expected total_with_amendment == amount")
	}

	if c.RemainingAmount != c.Amount {
		t.Error("expected remaining_amount == amount")
	}

	if !c.IsActive {
		t.Error("new contract must be active")
	}
}

func TestCreateContract_WithOptionalFields(t *testing.T) {
	sid := insertSupplier(t)

	cur := "KZT"
	acur := 100000.0
	bal := 50000.0
	signedDate := time.Now().Truncate(24 * time.Hour)

	resp, err := CreateContract(
		admCtx(),
		sid,
		&CreateContractRequest{
			ContractNumber:   uniqueContractNumber(),
			VatFlag:          0,
			SignedDate:       signedDate,
			Amount:           200000,
			Currency:         &cur,
			AmountCurrency:   &acur,
			BalanceAtYearEnd: &bal,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c := resp.Contract

	if c.Currency == nil || *c.Currency != cur {
		t.Errorf("expected currency %q", cur)
	}

	if c.AmountCurrency == nil || *c.AmountCurrency != acur {
		t.Errorf("expected amount_currency %v", acur)
	}

	if c.BalanceAtYearEnd == nil || *c.BalanceAtYearEnd != bal {
		t.Errorf("expected balance_at_year_end %v", bal)
	}
}

func TestCreateContract_EmptyContractNumber(t *testing.T) {
	sid := insertSupplier(t)

	_, err := CreateContract(
		admCtx(),
		sid,
		&CreateContractRequest{
			ContractNumber: "   ",
			VatFlag:        0,
			SignedDate:     time.Now(),
			Amount:         1000,
		},
	)

	if err == nil {
		t.Fatal("expected error")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument got %v", errs.Code(err))
	}
}

func TestCreateContract_NegativeAmount(t *testing.T) {
	sid := insertSupplier(t)

	_, err := CreateContract(
		admCtx(),
		sid,
		&CreateContractRequest{
			ContractNumber: uniqueContractNumber(),
			VatFlag:        0,
			SignedDate:     time.Now(),
			Amount:         -1,
		},
	)

	if err == nil {
		t.Fatal("expected error")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument got %v", errs.Code(err))
	}
}

func TestCreateContract_EndDateBeforeSignedDate(t *testing.T) {
	sid := insertSupplier(t)

	signed := time.Now()
	end := signed.Add(-24 * time.Hour)

	_, err := CreateContract(
		admCtx(),
		sid,
		&CreateContractRequest{
			ContractNumber: uniqueContractNumber(),
			VatFlag:        0,
			SignedDate:     signed,
			EndDate:        &end,
			Amount:         1000,
		},
	)

	if err == nil {
		t.Fatal("expected error")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument got %v", errs.Code(err))
	}
}

func TestCreateContract_EMPDenied(t *testing.T) {
	_, err := CreateContract(
		empCtx(),
		uuid.New().String(),
		&CreateContractRequest{
			ContractNumber: uniqueContractNumber(),
			VatFlag:        0,
			SignedDate:     time.Now(),
			Amount:         1000,
		},
	)

	if err == nil {
		t.Fatal("expected error")
	}

	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf("expected PermissionDenied got %v", errs.Code(err))
	}
}

// ════ GET ════

func TestGetContract_Success(t *testing.T) {
	sid := insertSupplier(t)
	c := makeContract(t, sid, 100000)

	resp, err := GetContract(admCtx(), c.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Contract.ID != c.ID {
		t.Errorf("expected %q got %q", c.ID, resp.Contract.ID)
	}
}

// ════ LIST ════

func TestListContracts_ReturnsCreated(t *testing.T) {
	sid := insertSupplier(t)
	c := makeContract(t, sid, 50000)

	resp, err := ListContracts(
		admCtx(),
		&ListContractsFilter{
			SupplierID: sid,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false

	for _, rc := range resp.Contracts {
		if rc.ID == c.ID {
			found = true
			break
		}
	}

	if !found {
		t.Error("created contract not found")
	}
}

// ════ UPDATE ════

func TestUpdateContract_ContractNumber(t *testing.T) {
	sid := insertSupplier(t)
	c := makeContract(t, sid, 100000)

	newNum := uniqueContractNumber()

	resp, err := UpdateContract(
		admCtx(),
		c.ID,
		&UpdateContractRequest{
			ContractNumber: &newNum,
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Contract.ContractNumber != newNum {
		t.Errorf(
			"expected %q got %q",
			newNum,
			resp.Contract.ContractNumber,
		)
	}
}

// ════ DELETE ════

func TestDeleteContract_Success(t *testing.T) {
	sid := insertSupplier(t)
	c := makeContract(t, sid, 75000)

	resp, err := DeleteContract(admCtx(), c.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

// ════ AMENDMENT ════

func TestAddAmendment_Success(t *testing.T) {
	sid := insertSupplier(t)
	c := makeContract(t, sid, 100000)

	amendDate := time.Now().Truncate(24 * time.Hour)
	extraAmount := 50000.0

	resp, err := AddAmendment(
		admCtx(),
		c.ID,
		&AmendmentRequest{
			AmendmentNumber: "ДС-001",
			AmendmentDate:   amendDate,
			AmendmentAmount: extraAmount,
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := resp.Contract

	if updated.AmendmentNumber == nil {
		t.Error("expected amendment number")
	}

	expected := c.Amount + extraAmount

	if updated.TotalWithAmendment != expected {
		t.Errorf(
			"expected total %v got %v",
			expected,
			updated.TotalWithAmendment,
		)
	}
}

// ════ STATUS COMPUTATION (via real data) ════

func TestCreateContract_StatusActive_NoEndDate(t *testing.T) {
	sid := insertSupplier(t)
	c := makeContract(t, sid, 1000)

	resp, err := GetContract(admCtx(), c.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Contract.Status != StatusActive {
		t.Errorf(
			"expected ACTIVE got %q",
			resp.Contract.Status,
		)
	}
}

// ════ FULL LIFECYCLE ════

func TestContract_FullLifecycle(t *testing.T) {
	sid := insertSupplier(t)

	signedDate := time.Now().Truncate(24 * time.Hour)

	createResp, err := CreateContract(
		admCtx(),
		sid,
		&CreateContractRequest{
			ContractNumber: uniqueContractNumber(),
			VatFlag:        20,
			SignedDate:     signedDate,
			Amount:         1000000,
		},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	id := createResp.Contract.ID

	_, err = AddAmendment(
		admCtx(),
		id,
		&AmendmentRequest{
			AmendmentNumber: "ДС-Lifecycle",
			AmendmentDate:   signedDate,
			AmendmentAmount: 250000,
		},
	)
	if err != nil {
		t.Fatalf("amendment: %v", err)
	}

	_, err = DeleteContract(admCtx(), id)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
}
