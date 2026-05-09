package contractssuppliers

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.app/db/ent/contractsupplier"
	csh "encore.app/orgstructure/contracts_suppliers_history"
	"encore.dev/beta/errs"
	"encore.dev/rlog"
	"encore.dev/storage/objects"
	"encore.dev/storage/sqldb"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

// ════ DATABASE ════

var (
	db     = sqldb.Named("lms")
	Client = newEntClient()

	// contractFiles stores uploaded contract documents (pdf/png/jpeg).
	// Keyed by "<contract_id>/<file_name>".
	contractFiles = objects.NewBucket("contract-files", objects.BucketConfig{})
)

func newEntClient() *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, db.Stdlib())
	return ent.NewClient(ent.Driver(drv))
}

var requirePermission = authhandler.RequirePermission

// ════ ENDPOINTS ════

// CreateContract creates a new supplier contract.
//
//encore:api auth method=POST path=/suppliers/:supplierID/contracts
func CreateContract(ctx context.Context, supplierID string, req *CreateContractRequest) (*GetContractResponse, error) {
	if _, err := requirePermission(); err != nil {
		return nil, err
	}

	if err := validateCreateRequest(req); err != nil {
		return nil, err
	}

	// TODO: validate supplier exists once suppliers module is merged into dev.

	row, err := insertContract(ctx, supplierID, req)
	if err != nil {
		return nil, err
	}

	newContract := entToContract(row)

	// Audit failure does not fail the request — the contract already exists.
	if auditErr := csh.InsertAuditRecord(ctx, newContract.ID, csh.OpCreate, nil, csh.EntToContract(row)); auditErr != nil {
		rlog.Error("contracts-suppliers: failed to write audit record",
			"contract_id", newContract.ID, "err", auditErr)
	}

	return &GetContractResponse{Contract: *newContract}, nil
}

// ListContracts returns a paginated, filtered list of supplier contracts.
//
//encore:api auth method=GET path=/contracts-suppliers
func ListContracts(ctx context.Context, filter *ListContractsFilter) (*ListContractsResponse, error) {
	if _, err := requirePermission(); err != nil {
		return nil, err
	}

	page, limit := applyFilterDefaults(filter.Page, filter.Limit)

	rows, total, err := queryContractsFiltered(ctx, filter, page, limit)
	if err != nil {
		return nil, err
	}

	contracts := make([]ContractSupplier, 0, len(rows))
	for _, r := range rows {
		contracts = append(contracts, *entToContract(r))
	}

	return &ListContractsResponse{
		Contracts: contracts,
		Total:     total,
		Page:      page,
		Limit:     limit,
	}, nil
}

// GetContract returns a single supplier contract by ID.
//
//encore:api auth method=GET path=/contracts-suppliers/id/:id
func GetContract(ctx context.Context, id string) (*GetContractResponse, error) {
	if _, err := requirePermission(); err != nil {
		return nil, err
	}

	row, err := queryContractByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &GetContractResponse{Contract: *entToContract(row)}, nil
}

// UpdateContract patches a supplier contract.
//
//encore:api auth method=PATCH path=/contracts-suppliers/id/:id
func UpdateContract(ctx context.Context, id string, req *UpdateContractRequest) (*GetContractResponse, error) {
	if _, err := requirePermission(); err != nil {
		return nil, err
	}

	if err := validateUpdateRequest(req); err != nil {
		return nil, err
	}

	rowBefore, err := queryContractByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !rowBefore.IsActive {
		return nil, errs.B().Code(errs.NotFound).Msg("contract not found").Err()
	}

	rowAfter, err := updateContract(ctx, rowBefore, req)
	if err != nil {
		return nil, err
	}

	if auditErr := csh.InsertAuditRecord(ctx, id, csh.OpUpdate,
		csh.EntToContract(rowBefore), csh.EntToContract(rowAfter)); auditErr != nil {
		rlog.Error("contracts-suppliers: failed to write audit record",
			"contract_id", id, "err", auditErr)
	}

	return &GetContractResponse{Contract: *entToContract(rowAfter)}, nil
}

// DeleteContract soft-deletes a supplier contract (sets is_active=false).
//
//encore:api auth method=DELETE path=/contracts-suppliers/id/:id
func DeleteContract(ctx context.Context, id string) (*DeleteContractResponse, error) {
	if _, err := requirePermission(); err != nil {
		return nil, err
	}

	rowBefore, err := queryContractByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !rowBefore.IsActive {
		return nil, errs.B().Code(errs.NotFound).Msg("contract not found").Err()
	}

	rowAfter, err := softDeleteContract(ctx, rowBefore)
	if err != nil {
		return nil, err
	}

	if auditErr := csh.InsertAuditRecord(ctx, id, csh.OpDelete,
		csh.EntToContract(rowBefore), csh.EntToContract(rowAfter)); auditErr != nil {
		rlog.Error("contracts-suppliers: failed to write audit record",
			"contract_id", id, "err", auditErr)
	}

	return &DeleteContractResponse{Message: "contract deleted"}, nil
}

// AddAmendment records an amendment (доп. соглашение) on the contract.
// Only one amendment is allowed per contract; a second attempt returns 409.
// Recomputes total_with_amendment and remaining_amount.
//
//encore:api auth method=POST path=/contracts-suppliers/id/:id/amendment
func AddAmendment(ctx context.Context, id string, req *AmendmentRequest) (*GetContractResponse, error) {
	if _, err := requirePermission(); err != nil {
		return nil, err
	}

	if err := validateAmendmentRequest(req); err != nil {
		return nil, err
	}

	rowBefore, err := queryContractByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !rowBefore.IsActive {
		return nil, errs.B().Code(errs.NotFound).Msg("contract not found").Err()
	}
	if rowBefore.AmendmentNumber != nil {
		return nil, errs.B().Code(errs.AlreadyExists).Msg("amendment already exists for this contract").Err()
	}

	rowAfter, err := applyAmendment(ctx, rowBefore, req)
	if err != nil {
		return nil, err
	}

	if auditErr := csh.InsertAuditRecord(ctx, id, csh.OpUpdate,
		csh.EntToContract(rowBefore), csh.EntToContract(rowAfter)); auditErr != nil {
		rlog.Error("contracts-suppliers: failed to write audit record",
			"contract_id", id, "err", auditErr)
	}

	return &GetContractResponse{Contract: *entToContract(rowAfter)}, nil
}

// UploadFile attaches a contract document (pdf/jpg/png) to the contract.
// Replaces any previously uploaded file. Max size 25 MB.
//
//encore:api auth method=POST path=/contracts-suppliers/id/:id/upload-file
func UploadFile(ctx context.Context, id string, req *UploadFileRequest) (*GetContractResponse, error) {
	if _, err := requirePermission(); err != nil {
		return nil, err
	}

	if err := validateUploadFileRequest(req); err != nil {
		return nil, err
	}

	rowBefore, err := queryContractByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !rowBefore.IsActive {
		return nil, errs.B().Code(errs.NotFound).Msg("contract not found").Err()
	}

	mimeType := http.DetectContentType(req.FileData)
	if !isAllowedMimeType(mimeType) {
		return nil, errs.B().Code(errs.InvalidArgument).
			Msgf("unsupported file type %q; allowed: pdf, png, jpeg", mimeType).Err()
	}

	newKey := buildFileKey(id, req.FileName)
	if err := uploadFileToBucket(ctx, newKey, req.FileData); err != nil {
		return nil, err
	}

	// If replacing a different key, remove the old object (best-effort).
	if rowBefore.FileKey != nil && *rowBefore.FileKey != newKey {
		if rmErr := contractFiles.Remove(ctx, *rowBefore.FileKey); rmErr != nil {
			rlog.Error("contracts-suppliers: failed to remove old file",
				"contract_id", id, "key", *rowBefore.FileKey, "err", rmErr)
		}
	}

	rowAfter, err := updateContractFileFields(ctx, rowBefore, newKey, req.FileName, int64(len(req.FileData)), mimeType)
	if err != nil {
		// Roll back the bucket upload to avoid orphaned objects.
		if rmErr := contractFiles.Remove(ctx, newKey); rmErr != nil {
			rlog.Error("contracts-suppliers: failed to remove orphaned file",
				"contract_id", id, "key", newKey, "err", rmErr)
		}
		return nil, err
	}

	if auditErr := csh.InsertAuditRecord(ctx, id, csh.OpUpdate,
		csh.EntToContract(rowBefore), csh.EntToContract(rowAfter)); auditErr != nil {
		rlog.Error("contracts-suppliers: failed to write audit record",
			"contract_id", id, "err", auditErr)
	}

	return &GetContractResponse{Contract: *entToContract(rowAfter)}, nil
}

// GetFileURL returns a short-lived signed URL to download the contract's file.
//
//encore:api auth method=GET path=/contracts-suppliers/id/:id/file-url
func GetFileURL(ctx context.Context, id string) (*FileURLResponse, error) {
	if _, err := requirePermission(); err != nil {
		return nil, err
	}

	row, err := queryContractByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !row.IsActive {
		return nil, errs.B().Code(errs.NotFound).Msg("contract not found").Err()
	}
	if row.FileKey == nil {
		return nil, errs.B().Code(errs.NotFound).Msg("no file uploaded for this contract").Err()
	}

	signed, err := contractFiles.SignedDownloadURL(ctx, *row.FileKey, objects.WithTTL(signedURLTTL))
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to generate signed url").Cause(err).Err()
	}

	resp := &FileURLResponse{
		URL:       signed.URL,
		ExpiresAt: time.Now().Add(signedURLTTL),
	}
	if row.FileName != nil {
		resp.FileName = *row.FileName
	}
	if row.FileMimeType != nil {
		resp.MimeType = *row.FileMimeType
	}
	return resp, nil
}

// ImportContracts bulk-imports contracts from CSV/XLSX.
//
//encore:api auth method=POST path=/contracts-suppliers/import
func ImportContracts(ctx context.Context) (*ImportResponse, error) {
	if _, err := requirePermission(); err != nil {
		return nil, err
	}
	return nil, errs.B().Code(errs.Unimplemented).Msg("ImportContracts not implemented").Err()
}

// ════ INTERNAL ════

const (
	defaultPage  = 1
	defaultLimit = 20
	maxLimit     = 100

	// expiringSoonWindow is the threshold for marking a contract EXPIRING_SOON.
	expiringSoonWindow = 30 * 24 * time.Hour
)

// computeStatus derives the lifecycle status from end_date.
// Contracts without end_date are treated as ACTIVE.
func computeStatus(now time.Time, endDate *time.Time) ContractStatus {
	if endDate == nil {
		return StatusActive
	}
	if !now.Before(*endDate) {
		return StatusExpired
	}
	if endDate.Sub(now) <= expiringSoonWindow {
		return StatusExpiringSoon
	}
	return StatusActive
}

// applyFilterDefaults normalizes page and limit: page >= 1, limit in [1, 100].
func applyFilterDefaults(page, limit int) (int, int) {
	if page < 1 {
		page = defaultPage
	}
	if limit < 1 {
		limit = defaultLimit
	} else if limit > maxLimit {
		limit = maxLimit
	}
	return page, limit
}

func queryContractsFiltered(ctx context.Context, filter *ListContractsFilter, page, limit int) ([]*ent.ContractSupplier, int, error) {
	q := Client.ContractSupplier.Query()

	if !filter.IncludeInactive {
		q = q.Where(contractsupplier.IsActive(true))
	}

	if filter.SupplierID != "" {
		sid, err := uuid.Parse(filter.SupplierID)
		if err != nil {
			return nil, 0, errs.B().Code(errs.InvalidArgument).Msg("invalid supplier_id format").Err()
		}
		q = q.Where(contractsupplier.SupplierIDEQ(sid))
	}

	if search := strings.TrimSpace(filter.Search); search != "" {
		q = q.Where(contractsupplier.ContractNumberContainsFold(search))
	}

	if status := strings.TrimSpace(filter.Status); status != "" {
		s := ContractStatus(status)
		if !s.IsValid() {
			return nil, 0, errs.B().Code(errs.InvalidArgument).Msg("invalid status; allowed: ACTIVE, EXPIRED, EXPIRING_SOON").Err()
		}
		now := time.Now()
		switch s {
		case StatusExpired:
			q = q.Where(contractsupplier.EndDateLTE(now))
		case StatusExpiringSoon:
			q = q.Where(
				contractsupplier.EndDateGT(now),
				contractsupplier.EndDateLTE(now.Add(expiringSoonWindow)),
			)
		case StatusActive:
			q = q.Where(contractsupplier.Or(
				contractsupplier.EndDateIsNil(),
				contractsupplier.EndDateGT(now.Add(expiringSoonWindow)),
			))
		}
	}

	if !filter.ExpiryDateFrom.IsZero() {
		q = q.Where(contractsupplier.EndDateGTE(filter.ExpiryDateFrom))
	}
	if !filter.ExpiryDateTo.IsZero() {
		q = q.Where(contractsupplier.EndDateLTE(filter.ExpiryDateTo))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, errs.B().Code(errs.Internal).Msg("failed to count contracts").Cause(err).Err()
	}

	rows, err := q.
		Order(ent.Desc(contractsupplier.FieldCreatedAt)).
		Limit(limit).
		Offset((page - 1) * limit).
		All(ctx)
	if err != nil {
		return nil, 0, errs.B().Code(errs.Internal).Msg("failed to list contracts").Cause(err).Err()
	}

	return rows, total, nil
}

func validateUpdateRequest(req *UpdateContractRequest) error {
	if req == nil {
		return errs.B().Code(errs.InvalidArgument).Msg("request body is required").Err()
	}
	if req.ContractNumber == nil && req.VatFlag == nil && req.SignedDate == nil && req.EndDate == nil &&
		req.AmountCurrency == nil && req.Currency == nil && req.BalanceAtYearEnd == nil {
		return errs.B().Code(errs.InvalidArgument).Msg("no fields to update").Err()
	}
	if req.ContractNumber != nil && strings.TrimSpace(*req.ContractNumber) == "" {
		return errs.B().Code(errs.InvalidArgument).Msg("contract_number cannot be empty").Err()
	}
	if req.SignedDate != nil && req.SignedDate.IsZero() {
		return errs.B().Code(errs.InvalidArgument).Msg("signed_date cannot be zero").Err()
	}
	if req.EndDate != nil && req.EndDate.IsZero() {
		return errs.B().Code(errs.InvalidArgument).Msg("end_date cannot be zero").Err()
	}
	if req.VatFlag != nil && (*req.VatFlag < 0 || *req.VatFlag > 100) {
		return errs.B().Code(errs.InvalidArgument).Msg("vat_flag must be between 0 and 100").Err()
	}
	if req.Amount != nil && *req.Amount < 0 {
		return errs.B().Code(errs.InvalidArgument).Msg("amount must be >= 0").Err()
	}

	return nil
}

func updateContract(ctx context.Context, row *ent.ContractSupplier, req *UpdateContractRequest) (*ent.ContractSupplier, error) {
	if req.Amount != nil && row.AmendmentAmount != nil {
		return nil, errs.B().
			Code(errs.FailedPrecondition).
			Msg("cannot update amount after amendment exists; use amendment flow").
			Err()
	}

	upd := Client.ContractSupplier.UpdateOne(row)

	if req.ContractNumber != nil {
		upd.SetContractNumber(strings.TrimSpace(*req.ContractNumber))
	}
	if req.VatFlag != nil {
		upd.SetVatFlag(*req.VatFlag)
	}
	if req.SignedDate != nil {
		upd.SetSignedDate(*req.SignedDate)
	}
	if req.EndDate != nil {
		upd.SetEndDate(*req.EndDate)
	}
	if req.Amount != nil {
		upd.SetAmount(*req.Amount)

		upd.SetTotalWithAmendment(*req.Amount)
		upd.SetRemainingAmount(*req.Amount)
	}
	if req.AmountCurrency != nil {
		upd.SetAmountCurrency(*req.AmountCurrency)
	}
	if req.Currency != nil {
		upd.SetCurrency(*req.Currency)
	}
	if req.BalanceAtYearEnd != nil {
		upd.SetBalanceAtYearEnd(*req.BalanceAtYearEnd)
	}

	updated, err := upd.Save(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to update contract").Cause(err).Err()
	}
	return updated, nil
}

const (
	maxUploadSize = 25 * 1024 * 1024
	signedURLTTL  = 15 * time.Minute
)

var allowedMimeTypes = map[string]struct{}{
	"application/pdf": {},
	"image/png":       {},
	"image/jpeg":      {},
}

func isAllowedMimeType(mime string) bool {
	_, ok := allowedMimeTypes[mime]
	return ok
}

func validateUploadFileRequest(req *UploadFileRequest) error {
	if req == nil {
		return errs.B().Code(errs.InvalidArgument).Msg("request body is required").Err()
	}
	if strings.TrimSpace(req.FileName) == "" {
		return errs.B().Code(errs.InvalidArgument).Msg("file_name is required").Err()
	}
	if len(req.FileData) == 0 {
		return errs.B().Code(errs.InvalidArgument).Msg("file_data is required").Err()
	}
	if len(req.FileData) > maxUploadSize {
		return errs.B().Code(errs.InvalidArgument).Msg("file_data exceeds 25 MB limit").Err()
	}
	return nil
}

// buildFileKey produces a deterministic bucket key: "<contract_id>/<basename>".
// Strips any directory components from the user-supplied file name.
func buildFileKey(contractID, fileName string) string {
	return contractID + "/" + filepath.Base(strings.TrimSpace(fileName))
}

func uploadFileToBucket(ctx context.Context, key string, data []byte) error {
	w := contractFiles.Upload(ctx, key)
	if _, err := w.Write(data); err != nil {
		w.Abort(err)
		return errs.B().Code(errs.Internal).Msg("failed to upload file").Cause(err).Err()
	}
	if err := w.Close(); err != nil {
		return errs.B().Code(errs.Internal).Msg("failed to finalize upload").Cause(err).Err()
	}
	return nil
}

func updateContractFileFields(ctx context.Context, row *ent.ContractSupplier, key, name string, size int64, mime string) (*ent.ContractSupplier, error) {
	updated, err := Client.ContractSupplier.UpdateOne(row).
		SetFileKey(key).
		SetFileName(name).
		SetFileSize(size).
		SetFileMimeType(mime).
		Save(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to save file metadata").Cause(err).Err()
	}
	return updated, nil
}

func validateAmendmentRequest(req *AmendmentRequest) error {
	if req == nil {
		return errs.B().Code(errs.InvalidArgument).Msg("request body is required").Err()
	}
	if strings.TrimSpace(req.AmendmentNumber) == "" {
		return errs.B().Code(errs.InvalidArgument).Msg("amendment_number is required").Err()
	}
	if req.AmendmentDate.IsZero() {
		return errs.B().Code(errs.InvalidArgument).Msg("amendment_date is required").Err()
	}
	// TODO: revisit once business confirms whether negative amendments (scope reduction) are needed.
	if req.AmendmentAmount <= 0 {
		return errs.B().Code(errs.InvalidArgument).Msg("amendment_amount must be > 0").Err()
	}
	return nil
}

func applyAmendment(ctx context.Context, row *ent.ContractSupplier, req *AmendmentRequest) (*ent.ContractSupplier, error) {
	updated, err := Client.ContractSupplier.UpdateOne(row).
		SetAmendmentNumber(strings.TrimSpace(req.AmendmentNumber)).
		SetAmendmentDate(req.AmendmentDate).
		SetAmendmentAmount(req.AmendmentAmount).
		SetTotalWithAmendment(row.Amount + req.AmendmentAmount).
		SetRemainingAmount(row.RemainingAmount + req.AmendmentAmount).
		Save(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to apply amendment").Cause(err).Err()
	}
	return updated, nil
}

func softDeleteContract(ctx context.Context, row *ent.ContractSupplier) (*ent.ContractSupplier, error) {
	updated, err := Client.ContractSupplier.
		UpdateOne(row).
		SetIsActive(false).
		Save(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to delete contract").Cause(err).Err()
	}
	return updated, nil
}

func queryContractByID(ctx context.Context, id string) (*ent.ContractSupplier, error) {
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
	return row, nil
}

func validateCreateRequest(req *CreateContractRequest) error {
	if req == nil {
		return errs.B().Code(errs.InvalidArgument).Msg("request body is required").Err()
	}
	if strings.TrimSpace(req.ContractNumber) == "" {
		return errs.B().Code(errs.InvalidArgument).Msg("contract_number is required").Err()
	}
	if req.Amount < 0 {
		return errs.B().Code(errs.InvalidArgument).Msg("amount must be >= 0").Err()
	}
	if req.VatFlag < 0 || req.VatFlag > 100 {
		return errs.B().Code(errs.InvalidArgument).Msg("vat_flag must be between 0 and 100").Err()
	}
	if req.SignedDate.IsZero() {
		return errs.B().Code(errs.InvalidArgument).Msg("signed_date is required").Err()
	}
	if req.EndDate != nil && !req.EndDate.After(req.SignedDate) {
		return errs.B().Code(errs.InvalidArgument).Msg("end_date must be after signed_date").Err()
	}
	return nil
}

func insertContract(ctx context.Context, supplierID string, req *CreateContractRequest) (*ent.ContractSupplier, error) {
	sid, err := uuid.Parse(supplierID)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid supplier_id format").Err()
	}

	row, err := Client.ContractSupplier.Create().
		SetSupplierID(sid).
		SetContractNumber(strings.TrimSpace(req.ContractNumber)).
		SetVatFlag(req.VatFlag).
		SetSignedDate(req.SignedDate).
		SetNillableEndDate(req.EndDate).
		SetAmount(req.Amount).
		SetNillableAmountCurrency(req.AmountCurrency).
		SetNillableCurrency(req.Currency).
		SetNillableBalanceAtYearEnd(req.BalanceAtYearEnd).
		SetTotalWithAmendment(req.Amount).
		SetRemainingAmount(req.Amount).
		SetIsActive(true).
		Save(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to create contract").Cause(err).Err()
	}
	return row, nil
}

func entToContract(e *ent.ContractSupplier) *ContractSupplier {
	return &ContractSupplier{
		ID:                 e.ID.String(),
		SupplierID:         e.SupplierID.String(),
		ContractNumber:     e.ContractNumber,
		VatFlag:            e.VatFlag,
		SignedDate:         e.SignedDate,
		EndDate:            e.EndDate,
		Status:             computeStatus(time.Now(), e.EndDate),
		Amount:             e.Amount,
		AmountCurrency:     e.AmountCurrency,
		Currency:           e.Currency,
		BalanceAtYearEnd:   e.BalanceAtYearEnd,
		AmendmentNumber:    e.AmendmentNumber,
		AmendmentDate:      e.AmendmentDate,
		AmendmentAmount:    e.AmendmentAmount,
		TotalWithAmendment: e.TotalWithAmendment,
		RemainingAmount:    e.RemainingAmount,
		FileKey:            e.FileKey,
		FileName:           e.FileName,
		FileSize:           e.FileSize,
		FileMimeType:       e.FileMimeType,
		IsActive:           e.IsActive,
		CreatedAt:          e.CreatedAt,
		UpdatedAt:          e.UpdatedAt,
	}
}
