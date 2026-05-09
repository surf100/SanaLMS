package suppliers

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.app/db/ent/supplier"
)

var (
	db     = sqldb.Named("lms")
	Client = newEntClient()
)

func newEntClient() *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, db.Stdlib())
	return ent.NewClient(ent.Driver(drv))
}

// ════ ENDPOINTS ════

// CreateSupplier creates supplier
//
//encore:api auth method=POST path=/suppliers
func CreateSupplier(ctx context.Context, req *CreateSupplierRequest) (*GetSupplierResponse, error) {
	ud, ok := auth.Data().(*authhandler.AuthData)
	if !ok || ud.CompanyID == "" {
		return nil, errs.B().Code(errs.PermissionDenied).Msg("missing company in token").Err()
	}

	if strings.TrimSpace(req.Name) == "" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("name is required").Err()
	}
	if !req.Type.IsValid() {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("type must be LEGAL or INDIVIDUAL").Err()
	}

	clientUUID, err := uuid.Parse(ud.CompanyID)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("invalid company id in token").Err()
	}

	s, err := insertSupplier(ctx, clientUUID, req)
	if err != nil {
		return nil, err
	}

	return &GetSupplierResponse{Supplier: *s}, nil
}

// ListSuppliers returns suppliers with optional filters:
//   - type: LEGAL | INDIVIDUAL
//   - is_active: true | false (defaults to true if omitted)
//   - search: case-insensitive substring match on name
//
//encore:api auth method=GET path=/suppliers
func ListSuppliers(ctx context.Context, params *ListSuppliersParams) (*ListSuppliersResponse, error) {
	var supplierType *SupplierType
	if params.Type != "" {
		t := SupplierType(params.Type)

		if !t.IsValid() {
			return nil, errs.B().
				Code(errs.InvalidArgument).
				Msg("type must be LEGAL or INDIVIDUAL").
				Err()
		}

		supplierType = &t
	}

	var isActive *bool
	if params.IsActive != "" {
		b, err := strconv.ParseBool(params.IsActive)
		if err != nil {
			return nil, errs.B().
				Code(errs.InvalidArgument).
				Msg("is_active must be true or false").
				Err()
		}
		isActive = &b
	}

	var search *string
	if s := strings.TrimSpace(params.Search); s != "" {
		search = &s
	}

	suppliers, err := querySuppliers(ctx, supplierType, isActive, search)
	if err != nil {
		return nil, err
	}

	return &ListSuppliersResponse{Suppliers: suppliers}, nil
}

// ListSuppliersWithBudget returns active suppliers with aggregated budget totals from contracts.
// Optional filters:
//   - type: LEGAL | INDIVIDUAL
//   - is_active: true | false (defaults to true if omitted)
//   - search: case-insensitive substring match on name
//
//encore:api auth method=GET path=/supplier/budget
func ListSuppliersWithBudget(ctx context.Context, params *ListSuppliersParams) (*ListSuppliersWithBudgetResponse, error) {
	var supplierType *SupplierType
	if params.Type != "" {
		t := SupplierType(params.Type)
		if !t.IsValid() {
			return nil, errs.B().
				Code(errs.InvalidArgument).
				Msg("type must be LEGAL or INDIVIDUAL").
				Err()
		}
		supplierType = &t
	}

	var isActive *bool
	if params.IsActive != "" {
		b, err := strconv.ParseBool(params.IsActive)
		if err != nil {
			return nil, errs.B().
				Code(errs.InvalidArgument).
				Msg("is_active must be true or false").
				Err()
		}
		isActive = &b
	}

	var search *string
	if s := strings.TrimSpace(params.Search); s != "" {
		search = &s
	}

	suppliers, err := querySuppliersWithBudget(ctx, supplierType, isActive, search)
	if err != nil {
		return nil, err
	}

	return &ListSuppliersWithBudgetResponse{Suppliers: suppliers}, nil
}

// GetSupplier returns supplier by id.
//
//encore:api auth method=GET path=/suppliers/:id
func GetSupplier(ctx context.Context, id string) (*GetSupplierResponse, error) {
	s, err := querySupplierByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &GetSupplierResponse{Supplier: *s}, nil
}

// UpdateSupplier partially updates supplier by id.
//
//encore:api auth method=PATCH path=/suppliers/:id
func UpdateSupplier(ctx context.Context, id string, req *UpdateSupplierRequest) (*GetSupplierResponse, error) {
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("name cannot be empty").Err()
	}
	if req.Type != nil && !req.Type.IsValid() {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("type must be LEGAL or INDIVIDUAL").Err()
	}

	s, err := updateSupplier(ctx, id, req)
	if err != nil {
		return nil, err
	}

	return &GetSupplierResponse{Supplier: *s}, nil
}

// DeleteSupplier soft delete of supplier (is_active = false).
//
//encore:api auth method=DELETE path=/suppliers/:id
func DeleteSupplier(ctx context.Context, id string) (*DeleteSupplierResponse, error) {
	if err := softDeleteSupplier(ctx, id); err != nil {
		return nil, err
	}

	return &DeleteSupplierResponse{Message: "supplier deleted successfully"}, nil
}

// UploadSuppliers validates uploaded suppliers .xlsx or .csv file before import.
//
//encore:api auth method=POST path=/suppliers-import/upload
func UploadSuppliers(ctx context.Context, req *UploadSuppliersRequest) (*UploadSuppliersResponse, error) {
	ud, ok := auth.Data().(*authhandler.AuthData)
	if !ok || ud.CompanyID == "" {
		return nil, errs.B().Code(errs.PermissionDenied).Msg("missing company in token").Err()
	}

	clientUID, err := uuid.Parse(ud.CompanyID)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("invalid company id in token").Err()
	}

	if req == nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("request body is required").Err()
	}
	if err := validateSupplierUploadRequest(req.FileName, req.FileData); err != nil {
		return nil, err
	}

	parsedRows, previewRows, validationErrors, totalRows, err := parseAndValidateSupplierFile(req.FileData, req.FileName)
	if err != nil {
		return nil, err
	}

	parsedRows, previewRows, validationErrors, err = applySupplierBusinessRules(ctx, clientUID, parsedRows, previewRows, validationErrors)
	if err != nil {
		return nil, err
	}

	validRows := len(parsedRows)

	return &UploadSuppliersResponse{
		IsValid:     len(validationErrors) == 0,
		TotalRows:   totalRows,
		ValidRows:   validRows,
		InvalidRows: totalRows - validRows,
		Errors:      validationErrors,
		Rows:        previewRows,
	}, nil
}

// ImportSuppliers imports suppliers from uploaded .xlsx or .csv file.
//
//encore:api auth method=POST path=/suppliers-import/import
func ImportSuppliers(ctx context.Context, req *ImportSuppliersRequest) (*ImportSuppliersResponse, error) {
	ud, ok := auth.Data().(*authhandler.AuthData)
	if !ok || ud.CompanyID == "" {
		return nil, errs.B().Code(errs.PermissionDenied).Msg("missing company in token").Err()
	}

	clientUID, err := uuid.Parse(ud.CompanyID)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("invalid company id in token").Err()
	}

	if req == nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("request body is required").Err()
	}
	if err := validateSupplierUploadRequest(req.FileName, req.FileData); err != nil {
		return nil, err
	}

	parsedRows, _, validationErrors, _, err := parseAndValidateSupplierFile(req.FileData, req.FileName)
	if err != nil {
		return nil, err
	}

	rowsToImport := filterSupplierRows(parsedRows, req.SelectedRows, validationErrors)
	if len(rowsToImport) == 0 {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("no valid rows selected for import").Err()
	}

	importedCount, err := bulkInsertSuppliers(ctx, clientUID, rowsToImport)
	if err != nil {
		return nil, err
	}

	return &ImportSuppliersResponse{
		ImportedCount: importedCount,
		Message:       fmt.Sprintf("imported %d suppliers", importedCount),
	}, nil
}

// ════ INTERNAL ════

func insertSupplier(ctx context.Context, clientID uuid.UUID, req *CreateSupplierRequest) (*Supplier, error) {
	builder := Client.Supplier.
		Create().
		SetClientID(clientID).
		SetType(supplier.Type(req.Type)).
		SetName(req.Name)

	if req.BinOrIIN != nil {
		builder = builder.SetBinOrIin(*req.BinOrIIN)
	}
	if req.LocalContentPct != nil {
		builder = builder.SetLocalContentPct(*req.LocalContentPct)
	}

	row, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, errs.B().Code(errs.AlreadyExists).Msg("supplier already exists").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to create supplier").Cause(err).Err()
	}

	return entToSupplier(row), nil
}

func querySupplierByID(ctx context.Context, id string) (*Supplier, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid id format").Err()
	}

	row, err := Client.Supplier.
		Query().
		Where(supplier.IDEQ(uid), supplier.IsActiveEQ(true)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("supplier not found").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to get supplier").Cause(err).Err()
	}

	return entToSupplier(row), nil
}

// querySuppliers builds a filtered query from optional filters.
// If isActive is nil, only active suppliers are returned by default.
// Type and search filters are applied only when provided.
func querySuppliers(
	ctx context.Context,
	supplierType *SupplierType,
	isActive *bool,
	search *string,
) ([]Supplier, error) {

	q := Client.Supplier.Query()

	//is_active
	if isActive != nil {
		q = q.Where(supplier.IsActiveEQ(*isActive))
	} else {
		// default = true
		q = q.Where(supplier.IsActiveEQ(true))
	}

	//type
	if supplierType != nil {
		q = q.Where(supplier.TypeEQ(supplier.Type(*supplierType)))
	}

	//search
	if search != nil {
		q = q.Where(supplier.NameContainsFold(*search))
	}

	rows, err := q.
		Order(ent.Asc(supplier.FieldName)).
		All(ctx)

	if err != nil {
		return nil, errs.B().
			Code(errs.Internal).
			Msg("failed to list suppliers").
			Cause(err).
			Err()
	}

	suppliers := make([]Supplier, 0, len(rows))
	for _, row := range rows {
		suppliers = append(suppliers, *entToSupplier(row))
	}

	return suppliers, nil
}

func updateSupplier(ctx context.Context, id string, req *UpdateSupplierRequest) (*Supplier, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid id format").Err()
	}

	builder := Client.Supplier.UpdateOneID(uid)

	if req.Name != nil {
		builder = builder.SetName(*req.Name)
	}
	if req.Type != nil {
		builder = builder.SetType(supplier.Type(*req.Type))
	}
	if req.BinOrIIN != nil {
		builder = builder.SetBinOrIin(*req.BinOrIIN)
	}
	if req.LocalContentPct != nil {
		builder = builder.SetLocalContentPct(*req.LocalContentPct)
	}
	if req.IsActive != nil {
		builder = builder.SetIsActive(*req.IsActive)
	}

	row, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("supplier not found").Err()
		}
		if ent.IsConstraintError(err) {
			return nil, errs.B().Code(errs.AlreadyExists).Msg("supplier already exists").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to update supplier").Cause(err).Err()
	}

	return entToSupplier(row), nil
}

func softDeleteSupplier(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return errs.B().Code(errs.InvalidArgument).Msg("invalid id format").Err()
	}

	exists, err := Client.Supplier.
		Query().
		Where(supplier.IDEQ(uid), supplier.IsActiveEQ(true)).
		Exist(ctx)
	if err != nil {
		return errs.B().Code(errs.Internal).Msg("failed to delete supplier").Cause(err).Err()
	}
	if !exists {
		return errs.B().Code(errs.NotFound).Msg("supplier not found").Err()
	}

	err = Client.Supplier.
		UpdateOneID(uid).
		SetIsActive(false).
		Exec(ctx)
	if err != nil {
		return errs.B().Code(errs.Internal).Msg("failed to delete supplier").Cause(err).Err()
	}

	return nil
}

var supplierRequiredHeaders = []string{"type", "name"}

func validateSupplierUploadRequest(fileName string, fileData []byte) error {
	if strings.TrimSpace(fileName) == "" {
		return errs.B().Code(errs.InvalidArgument).Msg("file_name is required").Err()
	}
	if len(fileData) == 0 {
		return errs.B().Code(errs.InvalidArgument).Msg("file_data is required").Err()
	}
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != ".xlsx" && ext != ".csv" {
		return errs.B().Code(errs.InvalidArgument).Msg("unsupported file format, use .xlsx or .csv").Err()
	}
	return nil
}

// parseAndValidateSupplierFile reads the file, checks headers, validates each row.
// Returns: valid parsed rows, all preview rows (including invalid), global errors, total row count.
func parseAndValidateSupplierFile(fileData []byte, fileName string) (
	[]parsedSupplierRow, []UploadSupplierRow, []string, int, error,
) {
	ext := strings.ToLower(filepath.Ext(fileName))

	var rawRows [][]string
	var err error

	switch ext {
	case ".csv":
		rawRows, err = parseSupplierCSV(fileData)
	case ".xlsx":
		rawRows, err = parseSupplierXLSX(fileData)
	}
	if err != nil {
		return nil, nil, nil, 0, errs.B().Code(errs.InvalidArgument).Msg("failed to parse file").Cause(err).Err()
	}

	if len(rawRows) < 2 {
		return nil, nil, []string{"file is empty or has only headers"}, 0, nil
	}

	// normalize headers and check required columns
	headerRow := rawRows[0]
	headerIndex, globalErr := buildSupplierHeaderIndex(headerRow)
	if globalErr != "" {
		return nil, nil, []string{globalErr}, 0, nil
	}

	dataRows := rawRows[1:]
	totalRows := len(dataRows)

	parsedRows := []parsedSupplierRow{}
	previewRows := []UploadSupplierRow{}
	globalErrors := []string{}

	for i, row := range dataRows {
		rowNum := i + 2 // +1 for header, +1 for 1-based indexing in error messages

		parsed, preview, rowErrors := validateSupplierRow(rowNum, row, headerIndex)
		preview.Errors = rowErrors
		preview.IsValid = len(rowErrors) == 0
		preview.Include = preview.IsValid

		previewRows = append(previewRows, preview)

		if preview.IsValid {
			parsedRows = append(parsedRows, *parsed)
		}
	}

	return parsedRows, previewRows, globalErrors, totalRows, nil
}

// buildSupplierHeaderIndex normalizes headers and returns map: header -> column index.
func buildSupplierHeaderIndex(headers []string) (map[string]int, string) {
	index := map[string]int{}
	for i, h := range headers {
		index[normalizeSupplierHeader(h)] = i
	}
	for _, required := range supplierRequiredHeaders {
		if _, ok := index[required]; !ok {
			return nil, fmt.Sprintf("missing required column: %s", required)
		}
	}
	return index, ""
}

// validateSupplierRow parses and validates one data row.
func validateSupplierRow(rowNum int, row []string, idx map[string]int) (*parsedSupplierRow, UploadSupplierRow, []string) {
	get := func(col string) string {
		i, ok := idx[col]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	preview := UploadSupplierRow{RowNumber: rowNum}
	errors := []string{}

	typRaw := get("type")
	supplierType := SupplierType(strings.ToUpper(typRaw))
	if typRaw == "" {
		errors = append(errors, "type is required")
	} else if !supplierType.IsValid() {
		errors = append(errors, fmt.Sprintf("type must be LEGAL or INDIVIDUAL, got: %s", typRaw))
	}

	name := get("name")
	if name == "" {
		errors = append(errors, "name is required")
	}

	var binOrIIN *string
	if v := get("bin_or_iin"); v != "" {
		binOrIIN = &v
	}

	var localContentPct *float64
	if v := get("local_content_pct"); v != "" {
		pct, err := strconv.ParseFloat(v, 64)
		if err != nil {
			errors = append(errors, "local_content_pct must be a number")
		} else if pct < 0 || pct > 100 {
			errors = append(errors, "local_content_pct must be between 0 and 100")
		} else {
			localContentPct = &pct
		}
	}

	preview.Type = string(supplierType)
	preview.Name = name
	preview.BinOrIIN = binOrIIN
	preview.LocalContentPct = localContentPct

	if len(errors) > 0 {
		return nil, preview, errors
	}

	return &parsedSupplierRow{
		RowNumber:       rowNum,
		Type:            supplierType,
		Name:            name,
		BinOrIIN:        binOrIIN,
		LocalContentPct: localContentPct,
	}, preview, errors
}

// filterSupplierRows returns rows for import considering SelectedRows.
// If SelectedRows is empty — we take all valid rows.
func filterSupplierRows(parsedRows []parsedSupplierRow, selectedRows []int, validationErrors []string) []parsedSupplierRow {
	if len(selectedRows) == 0 {
		return parsedRows
	}
	selectedSet := make(map[int]struct{}, len(selectedRows))
	for _, r := range selectedRows {
		selectedSet[r] = struct{}{}
	}
	filtered := []parsedSupplierRow{}
	for _, row := range parsedRows {
		if _, ok := selectedSet[row.RowNumber]; ok {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

// bulkInsertSuppliers creates suppliers in bulk using Ent's CreateBulk.
func bulkInsertSuppliers(ctx context.Context, clientID uuid.UUID, rows []parsedSupplierRow) (int, error) {
	builders := make([]*ent.SupplierCreate, 0, len(rows))

	for _, row := range rows {
		b := Client.Supplier.
			Create().
			SetClientID(clientID).
			SetType(supplier.Type(row.Type)).
			SetName(row.Name)

		if row.BinOrIIN != nil {
			b = b.SetBinOrIin(*row.BinOrIIN)
		}
		if row.LocalContentPct != nil {
			b = b.SetLocalContentPct(*row.LocalContentPct)
		}

		builders = append(builders, b)
	}

	created, err := Client.Supplier.CreateBulk(builders...).Save(ctx)
	if err != nil {
		return 0, errs.B().Code(errs.Internal).Msg("failed to import suppliers").Cause(err).Err()
	}

	return len(created), nil
}

func parseSupplierCSV(data []byte) ([][]string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true
	return reader.ReadAll()
}

func parseSupplierXLSX(data []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("xlsx file has no sheets")
	}

	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, err
	}

	for i, row := range rows {
		for j, cell := range row {
			rows[i][j] = normalizeExcelValue(cell)
		}
	}

	return rows, nil
}

func normalizeSupplierHeader(h string) string {
	return strings.ToLower(strings.TrimSpace(h))
}

func normalizeExcelValue(val string) string {
	val = strings.TrimSpace(val)
	if strings.ContainsAny(val, "eE") {
		f, err := strconv.ParseFloat(val, 64)
		if err == nil {
			return strconv.FormatInt(int64(f), 10)
		}
	}
	return val
}

// applySupplierBusinessRules checks DB-level constraints for each parsed row.
// Marks rows with existing bin_or_iin as invalid.
func applySupplierBusinessRules(
	ctx context.Context,
	clientID uuid.UUID,
	parsedRows []parsedSupplierRow,
	previewRows []UploadSupplierRow,
	validationErrors []string,
) ([]parsedSupplierRow, []UploadSupplierRow, []string, error) {
	bins := []string{}
	for _, row := range parsedRows {
		if row.BinOrIIN != nil {
			bins = append(bins, *row.BinOrIIN)
		}
	}

	// check for duplicates in DB for bin_or_iin values from file
	existingBins := map[string]struct{}{}
	if len(bins) > 0 {
		existing, err := Client.Supplier.
			Query().
			Where(
				supplier.ClientIDEQ(clientID),
				supplier.BinOrIinIn(bins...),
				// supplier.IsActiveEQ(true),
			).
			All(ctx)
		if err != nil {
			return nil, nil, nil, errs.B().Code(errs.Internal).Msg("failed to check existing suppliers").Cause(err).Err()
		}
		for _, s := range existing {
			if s.BinOrIin != nil {
				existingBins[*s.BinOrIin] = struct{}{}
			}
		}
	}

	// checking for duplicates within the file
	seenBins := map[string]struct{}{}

	validParsed := []parsedSupplierRow{}
	for i, row := range parsedRows {
		rowErrors := []string{}

		if row.BinOrIIN != nil {
			bin := *row.BinOrIIN

			if _, exists := existingBins[bin]; exists {
				rowErrors = append(rowErrors, fmt.Sprintf("bin_or_iin %s already exists in database", bin))
			}
			if _, seen := seenBins[bin]; seen {
				rowErrors = append(rowErrors, fmt.Sprintf("bin_or_iin %s is duplicated in file", bin))
			}
			seenBins[bin] = struct{}{}
		}

		if len(rowErrors) > 0 {
			previewRows[i].IsValid = false
			previewRows[i].Include = false
			previewRows[i].Errors = append(previewRows[i].Errors, rowErrors...)
		} else {
			validParsed = append(validParsed, row)
		}
	}

	return validParsed, previewRows, validationErrors, nil
}

func querySuppliersWithBudget(
	ctx context.Context,
	supplierType *SupplierType,
	isActive *bool,
	search *string,
) ([]SupplierWithBudget, error) {
	// Build WHERE clause dynamically
	conditions := []string{"1=1"}
	args := []any{}
	argIdx := 1

	if isActive != nil {
		conditions = append(conditions, fmt.Sprintf("s.is_active = $%d", argIdx))
		args = append(args, *isActive)
		argIdx++
	} else {
		conditions = append(conditions, "s.is_active = TRUE")
	}

	if supplierType != nil {
		conditions = append(conditions, fmt.Sprintf("s.type = $%d", argIdx))
		args = append(args, string(*supplierType))
		argIdx++
	}

	if search != nil {
		conditions = append(conditions, fmt.Sprintf("s.name ILIKE $%d", argIdx))
		args = append(args, "%"+*search+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	query := fmt.Sprintf(`
		SELECT
			s.id,
			s.client_id,
			s.type,
			s.name,
			s.bin_or_iin,
			s.local_content_pct,
			s.is_active,
			COALESCE(SUM(cs.total_with_amendment), 0)                            AS budget_total,
			COALESCE(SUM(cs.total_with_amendment) - SUM(cs.remaining_amount), 0) AS budget_used,
			COALESCE(SUM(cs.remaining_amount), 0)                                AS budget_remaining
		FROM suppliers s
		LEFT JOIN contract_suppliers cs ON cs.supplier_id = s.id AND cs.is_active = TRUE
		WHERE %s
		GROUP BY s.id, s.client_id, s.type, s.name, s.bin_or_iin, s.local_content_pct, s.is_active
		ORDER BY s.name ASC
	`, whereClause)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to list suppliers with budget").Cause(err).Err()
	}
	defer rows.Close()

	result := []SupplierWithBudget{}
	for rows.Next() {
		var s SupplierWithBudget

		if err := rows.Scan(
			&s.ID,
			&s.ClientID,
			&s.Type,
			&s.Name,
			&s.BinOrIIN,
			&s.LocalContentPct,
			&s.IsActive,
			&s.BudgetTotal,
			&s.BudgetUsed,
			&s.BudgetRemaining,
		); err != nil {
			return nil, errs.B().Code(errs.Internal).Msg("failed to scan supplier with budget").Cause(err).Err()
		}

		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to iterate suppliers with budget").Cause(err).Err()
	}

	return result, nil
}

// ════ HELPERS ════

func entToSupplier(e *ent.Supplier) *Supplier {
	var clientID *string
	if e.ClientID != uuid.Nil {
		str := e.ClientID.String()
		clientID = &str
	}

	return &Supplier{
		ID:              e.ID.String(),
		ClientID:        clientID,
		Type:            SupplierType(e.Type),
		Name:            e.Name,
		BinOrIIN:        e.BinOrIin,
		LocalContentPct: e.LocalContentPct,
		IsActive:        e.IsActive,
	}
}
