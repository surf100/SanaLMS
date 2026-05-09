// Package employees tests.
//
// This file imports encore.dev/storage/sqldb and cannot be run with plain go test.
// Use encore test ./orgstructure/employees/... to run these tests.
package employees

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
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

// makeDzo inserts a DZO directly so tests don't depend on the dzo service.
func testClientID(t *testing.T) uuid.UUID {
	return makeClient(t)
}

func makeClient(t *testing.T) uuid.UUID {
	t.Helper()

	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")

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

func makeDzo(t *testing.T) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := Client.DzoOrganization.Create().
		SetID(id).
		SetClientID(testClientID(t)).
		SetName("Test DZO " + uuid.NewString()).
		Save(ctx())
	if err != nil {
		t.Fatalf("makeDzo: %v", err)
	}
	return id
}

func makeDzoName(t *testing.T) string {
	t.Helper()

	name := "testDzo-" + uuid.NewString()
	_, err := Client.DzoOrganization.Create().
		SetClientID(testClientID(t)).
		SetName(name).
		Save(ctx())
	if err != nil {
		t.Fatalf("makeDzoName: %v", err)
	}
	return name
}

// makeEmployee bypasses CreateEmployee auth by calling insertEmployee directly.
func makeEmployee(t *testing.T, dzoID uuid.UUID, email string) *Employee {
	t.Helper()

	clientID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	kcUserID := "test-kc-" + uuid.NewString()

	userRow, err := Client.User.Create().
		SetKeycloakUserID(kcUserID).
		SetEmail(email).
		SetRole(string(authhandler.RoleEMP)).
		SetDzoID(dzoID).
		SetClientID(clientID).
		SetIsActive(true).
		Save(ctx())
	if err != nil {
		t.Fatalf("makeEmployee user: %v", err)
	}

	empRow, err := Client.Employee.Create().
		SetClientID(clientID).
		SetDzoID(dzoID).
		SetFullName("Test Employee").
		SetEmail(email).
		SetUserID(userRow.ID).
		SetIsDeleted(false).
		Save(ctx())
	if err != nil {
		t.Fatalf("makeEmployee employee: %v", err)
	}

	return entToEmployee(empRow)
}

// ════ VALIDATE EMAIL ════

func TestValidateEmail_Valid(t *testing.T) {
	if err := validateEmail("user@example.com"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateEmail_NoAt(t *testing.T) {
	err := validateEmail("notanemail")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestValidateEmail_MultipleAt(t *testing.T) {
	err := validateEmail("a@b@c.com")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestValidateEmail_NoDomainDot(t *testing.T) {
	err := validateEmail("user@nodot")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestValidateEmail_Empty(t *testing.T) {
	err := validateEmail("")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestValidateEmail_WhitespaceOnly(t *testing.T) {
	err := validateEmail("   ")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ════ GET ════

func TestGetEmployee_NotFound(t *testing.T) {
	_, err := GetEmployee(ctx(), "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestGetEmployee_InvalidID(t *testing.T) {
	_, err := GetEmployee(ctx(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestGetEmployee_Success(t *testing.T) {
	dzoID := makeDzo(t)
	emp := makeEmployee(t, dzoID, "get@example.com")

	resp, err := GetEmployee(ctx(), emp.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Employee.ID != emp.ID {
		t.Errorf("expected ID %q, got %q", emp.ID, resp.Employee.ID)
	}
	if resp.Employee.Email != emp.Email {
		t.Errorf("expected email %q, got %q", emp.Email, resp.Employee.Email)
	}
}

// ════ LIST ════

func TestListEmployees_SearchByFullName(t *testing.T) {
	dzoID := makeDzo(t)
	_ = makeEmployee(t, dzoID, "srchname@example.com")
	resp, err := ListEmployees(ctx(), &ListEmployeesParams{Search: "Test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Employees) == 0 {
		t.Error("expected at least one result for full_name search")
	}
}

func TestListEmployees_SearchByEmail(t *testing.T) {
	dzoID := makeDzo(t)
	makeEmployee(t, dzoID, "unique_domain@searchtest.io")

	resp, err := ListEmployees(ctx(), &ListEmployeesParams{Search: "searchtest.io"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Employees) == 0 {
		t.Error("expected at least one result for email search")
	}
}

func TestListEmployees_ReturnsEmptySliceNotNil(t *testing.T) {
	resp, err := ListEmployees(ctx(), &ListEmployeesParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Employees == nil {
		t.Error("expected []Employee{}, got nil")
	}
}

// ════ PATCH ════

func TestPatchEmployee_UpdatesOnlyProvidedFields(t *testing.T) {
	dzoID := makeDzo(t)
	emp := makeEmployee(t, dzoID, "patch@example.com")

	newName := "Updated Name"
	resp, err := PatchEmployee(ctx(), emp.ID, &UpdateEmployeeRequest{FullName: &newName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Employee.FullName != newName {
		t.Errorf("expected name %q, got %q", newName, resp.Employee.FullName)
	}
	if resp.Employee.Email != emp.Email {
		t.Error("email should be unchanged after patching only full_name")
	}
}

func TestPatchEmployee_DuplicateEmail(t *testing.T) {
	dzoID := makeDzo(t)
	makeEmployee(t, dzoID, "first@example.com")
	second := makeEmployee(t, dzoID, "second@example.com")

	dup := "first@example.com"
	_, err := PatchEmployee(ctx(), second.ID, &UpdateEmployeeRequest{Email: &dup})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.AlreadyExists {
		t.Errorf("expected AlreadyExists, got %v", errs.Code(err))
	}
}

func TestPatchEmployee_SameEmailAllowed(t *testing.T) {
	dzoID := makeDzo(t)
	emp := makeEmployee(t, dzoID, "same@example.com")

	sameEmail := emp.Email
	_, err := PatchEmployee(ctx(), emp.ID, &UpdateEmployeeRequest{Email: &sameEmail})
	if err != nil {
		t.Errorf("patching with same email should be allowed: %v", err)
	}
}

func TestPatchEmployee_InvalidEmail(t *testing.T) {
	dzoID := makeDzo(t)
	emp := makeEmployee(t, dzoID, "valid@example.com")

	bad := "notanemail"
	_, err := PatchEmployee(ctx(), emp.ID, &UpdateEmployeeRequest{Email: &bad})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestPatchEmployee_NotFound(t *testing.T) {
	newName := "Ghost"
	_, err := PatchEmployee(ctx(), "00000000-0000-0000-0000-000000000000", &UpdateEmployeeRequest{FullName: &newName})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

// ════ DELETE ════

func TestDeleteEmployee_NotFound(t *testing.T) {
	_, err := DeleteEmployee(ctx(), "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestDeleteEmployee_DoubleDelete(t *testing.T) {
	dzoID := makeDzo(t)
	emp := makeEmployee(t, dzoID, "doubledel@example.com")

	if _, err := DeleteEmployee(ctx(), emp.ID); err != nil {
		t.Fatalf("first delete failed: %v", err)
	}
	_, err := DeleteEmployee(ctx(), emp.ID)
	if err == nil {
		t.Fatal("expected error on second delete, got nil")
	}
	if errs.Code(err) != errs.NotFound {
		t.Errorf("expected NotFound on second delete, got %v", errs.Code(err))
	}
}

func makeEmployeesXLSX(t *testing.T, rows [][]string) []byte {
	t.Helper()

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	sheet := f.GetSheetName(0)
	for i, row := range rows {
		for j, value := range row {
			cell, err := excelize.CoordinatesToCellName(j+1, i+1)
			if err != nil {
				t.Fatalf("CoordinatesToCellName: %v", err)
			}
			if err := f.SetCellValue(sheet, cell, value); err != nil {
				t.Fatalf("SetCellValue: %v", err)
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer: %v", err)
	}
	return buf.Bytes()
}

func TestValidateUploadRequest_Success(t *testing.T) {
	data := makeEmployeesXLSX(t, [][]string{{"dzo_name", "full_name", "email"}})
	if err := validateUploadRequest("employees.xlsx", data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateUploadRequest_InvalidExtension(t *testing.T) {
	err := validateUploadRequest("employees.csv", []byte("abc"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestParseAndValidateEmployeeExcel_Valid(t *testing.T) {
	dzoID := uuid.New().String()
	data := makeEmployeesXLSX(t, [][]string{
		{"dzo_name", "full_name", "email", "department"},
		{dzoID, "John Doe", "john.doe@example.com", "IT"},
	})

	rows, previewRows, rowErrors, totalRows, err := parseAndValidateEmployeeExcel(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if totalRows != 1 {
		t.Fatalf("expected totalRows=1, got %d", totalRows)
	}
	if len(rowErrors) != 0 {
		t.Fatalf("expected no row errors, got: %v", rowErrors)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 parsed row, got %d", len(rows))
	}
	if len(previewRows) != 1 {
		t.Fatalf("expected 1 preview row, got %d", len(previewRows))
	}
	if !previewRows[0].IsValid || !previewRows[0].Include {
		t.Fatalf("expected valid preview row with include=true, got %+v", previewRows[0])
	}
	if rows[0].FullName != "John Doe" {
		t.Errorf("expected full_name John Doe, got %q", rows[0].FullName)
	}
}

func TestParseAndValidateEmployeeExcel_MissingRequiredHeader(t *testing.T) {
	data := makeEmployeesXLSX(t, [][]string{{"dzo_name", "full_name"}})

	_, _, _, _, err := parseAndValidateEmployeeExcel(data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestParseAndValidateEmployeeExcel_InvalidRowData(t *testing.T) {
	data := makeEmployeesXLSX(t, [][]string{
		{"dzo_name", "full_name", "email", "birth_date", "user_id"},
		{"not-uuid", "", "not-email", "31-12-2020", "bad-user"},
	})

	rows, previewRows, rowErrors, totalRows, err := parseAndValidateEmployeeExcel(data)
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if totalRows != 1 {
		t.Fatalf("expected totalRows=1, got %d", totalRows)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 valid rows, got %d", len(rows))
	}
	if len(previewRows) != 1 {
		t.Fatalf("expected 1 preview row, got %d", len(previewRows))
	}
	if previewRows[0].IsValid || previewRows[0].Include {
		t.Fatalf("expected invalid preview row with include=false, got %+v", previewRows[0])
	}
	if len(rowErrors) < 4 {
		t.Fatalf("expected several validation errors, got %v", rowErrors)
	}
}

func TestParseAndValidateEmployeeExcel_InvalidFile(t *testing.T) {
	_, _, _, _, err := parseAndValidateEmployeeExcel(bytes.Repeat([]byte("x"), 16))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestApplyUploadBusinessRules_DuplicateEmailsInFile(t *testing.T) {
	dzo := makeDzoName(t)

	parsedRows := []parsedEmployeeRow{
		{RowNumber: 2, DzoName: dzo, FullName: "A", Email: "dup@example.com"},
		{RowNumber: 3, DzoName: dzo, FullName: "B", Email: "DUP@example.com"},
		{RowNumber: 4, DzoName: dzo, FullName: "C", Email: "unique@example.com"},
	}
	previewRows := []UploadEmployeeRow{
		{RowNumber: 2, DzoName: dzo, Email: "dup@example.com", IsValid: true, Include: true, Errors: []string{}},
		{RowNumber: 3, DzoName: dzo, Email: "DUP@example.com", IsValid: true, Include: true, Errors: []string{}},
		{RowNumber: 4, DzoName: dzo, Email: "unique@example.com", IsValid: true, Include: true, Errors: []string{}},
	}

	filtered, preview, validationErrors, err := applyUploadBusinessRules(context.Background(), parsedRows, previewRows, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(filtered) != 1 {
		t.Fatalf("expected 1 valid row after duplicate filtering, got %d", len(filtered))
	}
	if filtered[0].RowNumber != 4 {
		t.Fatalf("expected only row 4 to remain, got row %d", filtered[0].RowNumber)
	}

	if preview[0].IsValid || preview[1].IsValid {
		t.Fatalf("expected duplicate-email rows to be invalid, got row2=%+v row3=%+v", preview[0], preview[1])
	}
	if !strings.Contains(strings.Join(preview[0].Errors, ";"), "duplicate email in file") {
		t.Fatalf("expected duplicate email error on row 2, got %v", preview[0].Errors)
	}
	if !strings.Contains(strings.Join(preview[1].Errors, ";"), "duplicate email in file") {
		t.Fatalf("expected duplicate email error on row 3, got %v", preview[1].Errors)
	}
	if len(validationErrors) != 2 {
		t.Fatalf("expected 2 validation errors for duplicates, got %d (%v)", len(validationErrors), validationErrors)
	}
}
