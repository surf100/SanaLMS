package contractsuppliershistory

import (
	"context"
	"testing"
	"time"

	"encore.app/auth/authhandler"
	"encore.dev/beta/errs"
)

func init() {
	requirePermission = func() (*authhandler.AuthData, error) {
		return &authhandler.AuthData{Role: authhandler.RoleADM}, nil
	}
}

// ════ HELPERS ════

func ctx() context.Context {
	return context.Background()
}

func validContract() *ContractSupplier {
	return &ContractSupplier{
		ID:                 "00000000-0000-0000-0000-000000000001",
		SupplierID:         "00000000-0000-0000-0000-000000000002",
		ContractNumber:     "№123/2025/1",
		VatFlag:            12,
		SignedDate:         time.Date(2025, 11, 10, 0, 0, 0, 0, time.UTC),
		Amount:             1000000,
		TotalWithAmendment: 1000000,
		RemainingAmount:    500000,
		IsActive:           true,
	}
}

// ════ OPERATION TYPE ════

func TestOperationType_IsValid(t *testing.T) {
	tests := []struct {
		name string
		op   OperationType
		want bool
	}{
		{"CREATE", OpCreate, true},
		{"UPDATE", OpUpdate, true},
		{"DELETE", OpDelete, true},
		{"invalid", OperationType("INVALID"), false},
		{"empty", OperationType(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.op.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ════ VALIDATE CONTRACT ════

func TestValidateContract_Valid(t *testing.T) {
	result := validateContract(validContract())
	if !result.IsValid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
}

func TestValidateContract_MissingContractNumber(t *testing.T) {
	cs := validContract()
	cs.ContractNumber = ""
	result := validateContract(cs)
	if result.IsValid {
		t.Error("expected invalid for empty contract_number")
	}
}

func TestValidateContract_NegativeAmount(t *testing.T) {
	cs := validContract()
	cs.Amount = -1
	result := validateContract(cs)
	if result.IsValid {
		t.Error("expected invalid for negative amount")
	}
}

func TestValidateContract_NegativeTotalWithAmendment(t *testing.T) {
	cs := validContract()
	cs.TotalWithAmendment = -100
	result := validateContract(cs)
	if result.IsValid {
		t.Error("expected invalid for negative total_with_amendment")
	}
}

func TestValidateContract_ZeroSignedDate(t *testing.T) {
	cs := validContract()
	cs.SignedDate = time.Time{}
	result := validateContract(cs)
	if result.IsValid {
		t.Error("expected invalid for zero signed_date")
	}
}

func TestValidateContract_EmptySupplierID(t *testing.T) {
	cs := validContract()
	cs.SupplierID = ""
	result := validateContract(cs)
	if result.IsValid {
		t.Error("expected invalid for empty supplier_id")
	}
}

func TestValidateContract_Inactive(t *testing.T) {
	cs := validContract()
	cs.IsActive = false
	result := validateContract(cs)
	if result.IsValid {
		t.Error("expected invalid for inactive contract")
	}
}

func TestValidateContract_MultipleErrors(t *testing.T) {
	cs := &ContractSupplier{}
	result := validateContract(cs)
	if result.IsValid {
		t.Error("expected invalid")
	}
	if len(result.Errors) < 3 {
		t.Errorf("expected multiple errors, got %d: %v", len(result.Errors), result.Errors)
	}
}

func TestValidateContract_NegativeRemainingAllowed(t *testing.T) {
	cs := validContract()
	cs.RemainingAmount = -18926902.37
	result := validateContract(cs)
	if !result.IsValid {
		t.Error("negative remaining_amount should be allowed (overspend)")
	}
}

// ════ BUILD SNAPSHOT ════

func TestBuildSnapshot_NilContract(t *testing.T) {
	snap := buildSnapshot(nil)
	if len(snap) != 0 {
		t.Errorf("expected empty map, got %v", snap)
	}
}

func TestBuildSnapshot_ValidContract(t *testing.T) {
	cs := validContract()
	snap := buildSnapshot(cs)

	if snap["id"] != cs.ID {
		t.Errorf("expected id %q, got %v", cs.ID, snap["id"])
	}
	if snap["contract_number"] != cs.ContractNumber {
		t.Errorf("expected contract_number %q, got %v", cs.ContractNumber, snap["contract_number"])
	}
	if snap["amount"] != cs.Amount {
		t.Errorf("expected amount %v, got %v", cs.Amount, snap["amount"])
	}
	if snap["vat_flag"] != cs.VatFlag {
		t.Errorf("expected vat_flag %v, got %v", cs.VatFlag, snap["vat_flag"])
	}
	if snap["is_active"] != cs.IsActive {
		t.Errorf("expected is_active %v, got %v", cs.IsActive, snap["is_active"])
	}
}

func TestBuildSnapshot_OptionalFields(t *testing.T) {
	amt := 500.0
	curr := "USD"
	amend := "№1-1"
	cs := validContract()
	cs.AmountCurrency = &amt
	cs.Currency = &curr
	cs.AmendmentNumber = &amend

	snap := buildSnapshot(cs)
	if snap["amount_currency"] != &amt {
		t.Error("expected amount_currency to be set")
	}
	if snap["currency"] != &curr {
		t.Error("expected currency to be set")
	}
	if snap["amendment_number"] != &amend {
		t.Error("expected amendment_number to be set")
	}
}

// ════ BUILD DIFF ════

func TestBuildDiff_NilOld(t *testing.T) {
	diff := buildDiff(nil, validContract())
	if len(diff) != 0 {
		t.Errorf("expected empty map, got %v", diff)
	}
}

func TestBuildDiff_NilNew(t *testing.T) {
	diff := buildDiff(validContract(), nil)
	if len(diff) != 0 {
		t.Errorf("expected empty map, got %v", diff)
	}
}

func TestBuildDiff_NoChanges(t *testing.T) {
	cs := validContract()
	diff := buildDiff(cs, cs)
	if len(diff) != 0 {
		t.Errorf("expected empty diff for identical contracts, got %v", diff)
	}
}

func TestBuildDiff_AmountChanged(t *testing.T) {
	old := validContract()
	new_ := validContract()
	new_.Amount = 2000000

	diff := buildDiff(old, new_)
	if _, ok := diff["amount"]; !ok {
		t.Error("expected 'amount' in diff")
	}
	entry := diff["amount"].(map[string]interface{})
	if entry["old"] != 1000000.0 || entry["new"] != 2000000.0 {
		t.Errorf("unexpected diff entry: %v", entry)
	}
}

func TestBuildDiff_VatFlagChanged(t *testing.T) {
	old := validContract()
	new_ := validContract()
	new_.VatFlag = 50

	diff := buildDiff(old, new_)
	if _, ok := diff["vat_flag"]; !ok {
		t.Error("expected 'vat_flag' in diff")
	}
}

func TestBuildDiff_MultipleFieldsChanged(t *testing.T) {
	old := validContract()
	new_ := validContract()
	new_.Amount = 2000000
	new_.VatFlag = 50
	new_.ContractNumber = "№456/2025/2"

	diff := buildDiff(old, new_)
	if len(diff) != 3 {
		t.Errorf("expected 3 changed fields, got %d: %v", len(diff), diff)
	}
}

func TestBuildDiff_OptionalFieldChanged(t *testing.T) {
	old := validContract()
	new_ := validContract()
	amt := 500.0
	new_.AmountCurrency = &amt

	diff := buildDiff(old, new_)
	if _, ok := diff["amount_currency"]; !ok {
		t.Error("expected 'amount_currency' in diff")
	}
}

func TestBuildDiff_OptionalFieldRemoved(t *testing.T) {
	amt := 500.0
	old := validContract()
	old.AmountCurrency = &amt
	new_ := validContract()

	diff := buildDiff(old, new_)
	if _, ok := diff["amount_currency"]; !ok {
		t.Error("expected 'amount_currency' in diff when removed")
	}
}

func TestBuildDiff_AmendmentDateChanged(t *testing.T) {
	old := validContract()
	new_ := validContract()
	d1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	old.AmendmentDate = &d1
	new_.AmendmentDate = &d2

	diff := buildDiff(old, new_)
	if _, ok := diff["amendment_date"]; !ok {
		t.Error("expected 'amendment_date' in diff")
	}
}

// ════ ENDPOINTS ════

func TestGetHistory_InvalidID(t *testing.T) {
	_, err := GetHistory(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestGetHistory_EmptyResult(t *testing.T) {
	resp, err := GetHistory(ctx(), "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Records == nil {
		t.Error("expected []HistoryRecord{}, got nil")
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 total, got %d", resp.Total)
	}
}

func TestValidateContract_Endpoint_InvalidID(t *testing.T) {
	_, err := ValidateContract(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestValidateContract_Endpoint_NotFound(t *testing.T) {
	_, err := ValidateContract(ctx(), "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

// ════ PTR HELPERS ════

func TestPtrStr(t *testing.T) {
	if ptrStr(nil) != "" {
		t.Error("expected empty string for nil")
	}
	s := "hello"
	if ptrStr(&s) != "hello" {
		t.Error("expected 'hello'")
	}
}

func TestPtrFloat(t *testing.T) {
	if ptrFloat(nil) != 0 {
		t.Error("expected 0 for nil")
	}
	f := 3.14
	if ptrFloat(&f) != 3.14 {
		t.Error("expected 3.14")
	}
}

func TestFormatDatePtr(t *testing.T) {
	if formatDatePtr(nil) != "" {
		t.Error("expected empty string for nil")
	}
	d := time.Date(2025, 11, 10, 0, 0, 0, 0, time.UTC)
	if formatDatePtr(&d) != "2025-11-10" {
		t.Errorf("expected '2025-11-10', got %q", formatDatePtr(&d))
	}
}
