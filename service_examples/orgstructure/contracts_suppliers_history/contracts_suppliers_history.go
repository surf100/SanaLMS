package contractsuppliershistory

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.app/db/ent/contractsupplier"
	"encore.app/db/ent/contractsupplierhistory"
	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

// ════ DATABASE ════

var (
	db     = sqldb.Named("lms")
	Client = newEntClient()
)

func newEntClient() *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, db.Stdlib())
	return ent.NewClient(ent.Driver(drv))
}

// ════ ENDPOINTS ════

// GetHistory returns the audit trail for a contract-supplier, sorted by changed_at DESC.
//
//encore:api auth method=GET path=/contracts-suppliers/id/:id/history
func GetHistory(ctx context.Context, id string) (*ListHistoryResponse, error) {
	if _, err := requirePermission(); err != nil {
		return nil, err
	}

	records, err := queryHistoryByContractID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &ListHistoryResponse{
		Records: records,
		Total:   len(records),
	}, nil
}

// ValidateContract validates a contract before it can be used in requests.
//
//encore:api auth method=GET path=/contracts-suppliers/id/:id/validate
func ValidateContract(ctx context.Context, id string) (*ValidateResponse, error) {
	if _, err := requirePermission(); err != nil {
		return nil, err
	}

	cs, err := queryContractByID(ctx, id)
	if err != nil {
		return nil, err
	}

	result := validateContract(cs)

	return &ValidateResponse{Result: result}, nil
}

// ════ INTERNAL ════

// InsertAuditRecord creates one history row after a successful mutation.
// This is the single entry point for audit logging from all mutation endpoints.
func InsertAuditRecord(ctx context.Context, contractID string, opType OperationType, oldContract, newContract *ContractSupplier) error {
	cid, err := uuid.Parse(contractID)
	if err != nil {
		return errs.B().Code(errs.InvalidArgument).Msg("invalid contract_id format").Err()
	}

	ad, _ := requirePermission()
	var changedBy *uuid.UUID
	if ad != nil {
		uid, parseErr := uuid.Parse(ad.KeycloakUserID)
		if parseErr == nil {
			changedBy = &uid
		}
	}

	snapshot := buildSnapshot(newContract)
	diff := buildDiff(oldContract, newContract)

	_, saveErr := Client.ContractSupplierHistory.
		Create().
		SetContractID(cid).
		SetOperationType(string(opType)).
		SetChangedAt(time.Now()).
		SetNillableChangedBy(changedBy).
		SetSnapshot(snapshot).
		SetDiff(diff).
		Save(ctx)
	if saveErr != nil {
		return errs.B().Code(errs.Internal).Msg("failed to write audit record").Cause(saveErr).Err()
	}

	return nil
}

var requirePermission = authhandler.RequirePermission

func queryContractByID(ctx context.Context, id string) (*ContractSupplier, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid contract id format").Err()
	}

	row, err := Client.ContractSupplier.
		Query().
		Where(contractsupplier.IDEQ(uid)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("contract not found").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to get contract").Cause(err).Err()
	}

	return EntToContract(row), nil
}

func queryHistoryByContractID(ctx context.Context, contractID string) ([]HistoryRecord, error) {
	cid, err := uuid.Parse(contractID)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid contract id format").Err()
	}

	rows, err := Client.ContractSupplierHistory.
		Query().
		Where(contractsupplierhistory.ContractIDEQ(cid)).
		Order(ent.Desc(contractsupplierhistory.FieldChangedAt)).
		All(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to query history").Cause(err).Err()
	}

	records := make([]HistoryRecord, 0, len(rows))
	for _, r := range rows {
		records = append(records, *entToHistoryRecord(r))
	}

	return records, nil
}

func validateContract(cs *ContractSupplier) ValidationResult {
	var errors []string

	if strings.TrimSpace(cs.ContractNumber) == "" {
		errors = append(errors, "contract_number is required")
	}
	if cs.Amount < 0 {
		errors = append(errors, "amount must be >= 0")
	}
	if cs.TotalWithAmendment < 0 {
		errors = append(errors, "total_with_amendment must be >= 0")
	}
	if cs.SignedDate.IsZero() {
		errors = append(errors, "signed_date is required")
	}
	if cs.SupplierID == "" {
		errors = append(errors, "supplier_id is required")
	}
	if !cs.IsActive {
		errors = append(errors, "contract is not active")
	}

	return ValidationResult{
		IsValid: len(errors) == 0,
		Errors:  errors,
	}
}

func buildSnapshot(cs *ContractSupplier) map[string]interface{} {
	if cs == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"id":                   cs.ID,
		"supplier_id":          cs.SupplierID,
		"contract_number":      cs.ContractNumber,
		"vat_flag":             cs.VatFlag,
		"signed_date":          cs.SignedDate.Format("2006-01-02"),
		"amount":               cs.Amount,
		"amount_currency":      cs.AmountCurrency,
		"currency":             cs.Currency,
		"balance_at_year_end":  cs.BalanceAtYearEnd,
		"amendment_number":     cs.AmendmentNumber,
		"amendment_date":       formatDatePtr(cs.AmendmentDate),
		"amendment_amount":     cs.AmendmentAmount,
		"total_with_amendment": cs.TotalWithAmendment,
		"remaining_amount":     cs.RemainingAmount,
		"is_active":            cs.IsActive,
	}
}

func buildDiff(old, new_ *ContractSupplier) map[string]interface{} {
	if old == nil || new_ == nil {
		return map[string]interface{}{}
	}

	diff := map[string]interface{}{}
	checkString(diff, "contract_number", old.ContractNumber, new_.ContractNumber)
	checkInt(diff, "vat_flag", old.VatFlag, new_.VatFlag)
	checkFloat(diff, "amount", old.Amount, new_.Amount)
	checkFloatPtr(diff, "amount_currency", old.AmountCurrency, new_.AmountCurrency)
	checkStringPtr(diff, "currency", old.Currency, new_.Currency)
	checkFloatPtr(diff, "balance_at_year_end", old.BalanceAtYearEnd, new_.BalanceAtYearEnd)
	checkStringPtr(diff, "amendment_number", old.AmendmentNumber, new_.AmendmentNumber)
	checkTimePtr(diff, "amendment_date", old.AmendmentDate, new_.AmendmentDate)
	checkFloatPtr(diff, "amendment_amount", old.AmendmentAmount, new_.AmendmentAmount)
	checkFloat(diff, "total_with_amendment", old.TotalWithAmendment, new_.TotalWithAmendment)
	checkFloat(diff, "remaining_amount", old.RemainingAmount, new_.RemainingAmount)
	checkBool(diff, "is_active", old.IsActive, new_.IsActive)

	return diff
}

func checkString(diff map[string]interface{}, field, old, new_ string) {
	if old != new_ {
		diff[field] = map[string]interface{}{"old": old, "new": new_}
	}
}

func checkBool(diff map[string]interface{}, field string, old, new_ bool) {
	if old != new_ {
		diff[field] = map[string]interface{}{"old": old, "new": new_}
	}
}

func checkInt(diff map[string]interface{}, field string, old, new_ int) {
	if old != new_ {
		diff[field] = map[string]interface{}{"old": old, "new": new_}
	}
}

func checkFloat(diff map[string]interface{}, field string, old, new_ float64) {
	if old != new_ {
		diff[field] = map[string]interface{}{"old": old, "new": new_}
	}
}

func checkStringPtr(diff map[string]interface{}, field string, old, new_ *string) {
	if ptrStr(old) != ptrStr(new_) {
		diff[field] = map[string]interface{}{"old": old, "new": new_}
	}
}

func checkFloatPtr(diff map[string]interface{}, field string, old, new_ *float64) {
	if ptrFloat(old) != ptrFloat(new_) {
		diff[field] = map[string]interface{}{"old": old, "new": new_}
	}
}

func checkTimePtr(diff map[string]interface{}, field string, old, new_ *time.Time) {
	oldStr := formatDatePtr(old)
	newStr := formatDatePtr(new_)
	if oldStr != newStr {
		diff[field] = map[string]interface{}{"old": oldStr, "new": newStr}
	}
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptrFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func formatDatePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// ════ HELPERS ════

func EntToContract(e *ent.ContractSupplier) *ContractSupplier {
	cs := &ContractSupplier{
		ID:                 e.ID.String(),
		SupplierID:         e.SupplierID.String(),
		ContractNumber:     e.ContractNumber,
		VatFlag:            e.VatFlag,
		SignedDate:         e.SignedDate,
		Amount:             e.Amount,
		TotalWithAmendment: e.TotalWithAmendment,
		RemainingAmount:    e.RemainingAmount,
		IsActive:           e.IsActive,
		CreatedAt:          e.CreatedAt,
		UpdatedAt:          e.UpdatedAt,
	}
	if e.AmountCurrency != nil {
		cs.AmountCurrency = e.AmountCurrency
	}
	if e.Currency != nil {
		cs.Currency = e.Currency
	}
	if e.BalanceAtYearEnd != nil {
		cs.BalanceAtYearEnd = e.BalanceAtYearEnd
	}
	if e.AmendmentNumber != nil {
		cs.AmendmentNumber = e.AmendmentNumber
	}
	if e.AmendmentDate != nil {
		cs.AmendmentDate = e.AmendmentDate
	}
	if e.AmendmentAmount != nil {
		cs.AmendmentAmount = e.AmendmentAmount
	}
	return cs
}

func entToHistoryRecord(e *ent.ContractSupplierHistory) *HistoryRecord {
	rec := &HistoryRecord{
		HistoryID:     e.HistoryID.String(),
		ContractID:    e.ContractID.String(),
		OperationType: OperationType(e.OperationType),
		ChangedAt:     e.ChangedAt,
	}
	if e.Snapshot != nil {
		rec.Snapshot, _ = json.Marshal(e.Snapshot)
	}
	if e.Diff != nil {
		rec.Diff, _ = json.Marshal(e.Diff)
	}
	if e.ChangedBy != nil {
		s := e.ChangedBy.String()
		rec.ChangedBy = &s
	}
	return rec
}
