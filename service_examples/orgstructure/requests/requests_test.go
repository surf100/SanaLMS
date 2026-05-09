package requests

import (
	"context"
	"testing"
	"time"

	"encore.app/auth/authhandler"
	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	encoreuuid "encore.dev/types/uuid"
	"github.com/google/uuid"
)

func ctx() context.Context {
	return context.Background()
}

func authCtxFor(keycloakUserID string, role authhandler.UserRole, dzoID *uuid.UUID) context.Context {
	ad := &authhandler.AuthData{
		KeycloakUserID: keycloakUserID,
		Role:           role,
	}
	if dzoID != nil {
		ad.DzoID = dzoID.String()
	}
	return auth.WithContext(context.Background(), auth.UID(keycloakUserID), ad)
}

func toEncoreUUID(id uuid.UUID) encoreuuid.UUID {
	return encoreuuid.UUID(id)
}

type requestTestFixture struct {
	clientID        string
	adminID         uuid.UUID
	adminKC         string
	hrOneID         uuid.UUID
	hrOneKC         string
	hrTwoID         uuid.UUID
	hrTwoKC         string
	hrThreeID       uuid.UUID
	hrThreeKC       string
	dzoOneID        uuid.UUID
	dzoTwoID        uuid.UUID
	dzoThreeID      uuid.UUID
	empOneID        uuid.UUID
	empTwoID        uuid.UUID
	trainingEventID uuid.UUID
	categoryID      uuid.UUID
	supplierID      uuid.UUID
	contractID      uuid.UUID
}

func newFixture(t *testing.T) *requestTestFixture {
	t.Helper()

	fx := &requestTestFixture{
		clientID:        uuid.NewString(),
		adminID:         uuid.New(),
		adminKC:         "kc-admin-" + uuid.NewString(),
		hrOneID:         uuid.New(),
		hrOneKC:         "kc-hr-one-" + uuid.NewString(),
		hrTwoID:         uuid.New(),
		hrTwoKC:         "kc-hr-two-" + uuid.NewString(),
		hrThreeID:       uuid.New(),
		hrThreeKC:       "kc-hr-three-" + uuid.NewString(),
		dzoOneID:        uuid.New(),
		dzoTwoID:        uuid.New(),
		dzoThreeID:      uuid.New(),
		empOneID:        uuid.New(),
		empTwoID:        uuid.New(),
		trainingEventID: uuid.New(),
		categoryID:      uuid.New(),
		supplierID:      uuid.New(),
		contractID:      uuid.New(),
	}

	ensureClient(t, fx.clientID)
	ensureDZO(t, fx.dzoOneID, fx.clientID, "DZO One")
	ensureDZO(t, fx.dzoTwoID, fx.clientID, "DZO Two")
	ensureDZO(t, fx.dzoThreeID, fx.clientID, "DZO Three")
	ensureCategory(t, fx.categoryID)
	ensureSupplier(t, fx.supplierID, fx.clientID)
	ensureContractSupplier(t, fx.contractID, fx.supplierID, 100000)
	ensureUser(t, fx.adminID, fx.adminKC, "admin@test.local", "ADM", fx.clientID, nil)
	ensureUser(t, fx.hrOneID, fx.hrOneKC, "hr1@test.local", "HR", fx.clientID, &fx.dzoOneID)
	ensureUser(t, fx.hrTwoID, fx.hrTwoKC, "hr2@test.local", "HR", fx.clientID, &fx.dzoTwoID)
	ensureUser(t, fx.hrThreeID, fx.hrThreeKC, "hr3@test.local", "HR", fx.clientID, &fx.dzoThreeID)
	ensureEmployee(t, fx.empOneID, fx.clientID, fx.dzoOneID, "Employee One", "emp1@test.local")
	ensureEmployee(t, fx.empTwoID, fx.clientID, fx.dzoTwoID, "Employee Two", "emp2@test.local")
	ensureTrainingEvent(t, fx.trainingEventID, fx.dzoOneID, fx.categoryID, fx.supplierID, fx.contractID, "External Learning")

	return fx
}

func ensureClient(t *testing.T, clientID string) {
	t.Helper()

	_, err := db.Exec(ctx(), `
		INSERT INTO clients (id, name, created_at, is_active)
		VALUES ($1, $2, NOW(), TRUE)
		ON CONFLICT (id) DO NOTHING
	`, mustUUID(t, clientID), "Client "+clientID[:8])
	if err != nil {
		t.Fatalf("insert client: %v", err)
	}
}

func ensureDZO(t *testing.T, dzoID uuid.UUID, clientID, name string) {
	t.Helper()

	_, err := db.Exec(ctx(), `
		INSERT INTO dzo_organizations (id, client_id, name, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, TRUE, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, dzoID, mustUUID(t, clientID), name)
	if err != nil {
		t.Fatalf("insert dzo: %v", err)
	}
}

func ensureUser(t *testing.T, userID uuid.UUID, keycloakID, email, role, clientID string, dzoID *uuid.UUID) {
	t.Helper()

	_, err := db.Exec(ctx(), `
		INSERT INTO users (id, keycloak_user_id, email, role, dzo_id, is_active, is_onboarded, created_at, updated_at, client_id)
		VALUES ($1, $2, $3, $4, $5, TRUE, TRUE, NOW(), NOW(), $6)
		ON CONFLICT (id) DO NOTHING
	`, userID, keycloakID, email, role, nullableUUIDForTest(dzoID), mustUUID(t, clientID))
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func ensureEmployee(t *testing.T, employeeID uuid.UUID, clientID string, dzoID uuid.UUID, fullName, email string) {
	t.Helper()

	_, err := db.Exec(ctx(), `
		INSERT INTO employees (id, client_id, dzo_id, full_name, email, is_active, is_deleted)
		VALUES ($1, $2, $3, $4, $5, TRUE, FALSE)
		ON CONFLICT (id) DO NOTHING
	`, employeeID, mustUUID(t, clientID), dzoID, fullName, email)
	if err != nil {
		t.Fatalf("insert employee: %v", err)
	}
}

func ensureTrainingEvent(t *testing.T, eventID, dzoID, categoryID, supplierID, contractID uuid.UUID, title string) {
	t.Helper()

	_, err := db.Exec(ctx(), `
		INSERT INTO training_events (
			id, title, start_date, end_date, location_type, location_city,
			category_id, dzo_id, participants_count, cost_per_person_vat,
			supplier_id, supplier_contract_id
		)
		VALUES ($1, $2, NOW(), NOW() + INTERVAL '1 day', 'OFFLINE', 'Astana',
			$3, $4, 0, 5000, $5, $6)
		ON CONFLICT (id) DO NOTHING
	`, eventID, title, categoryID, dzoID, supplierID, contractID)
	if err != nil {
		t.Fatalf("insert training event: %v", err)
	}
}

func ensureCategory(t *testing.T, categoryID uuid.UUID) {
	t.Helper()

	_, err := db.Exec(ctx(), `
		INSERT INTO categories (id, name, description)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO NOTHING
	`, categoryID, "Test Category", "Test category for budget tests")
	if err != nil {
		t.Fatalf("insert category: %v", err)
	}
}

func ensureSupplier(t *testing.T, supplierID uuid.UUID, clientID string) {
	t.Helper()

	bin := uuid.New().String()[:12] // ← УНИКАЛЬНЫЙ

	_, err := db.Exec(ctx(), `
		INSERT INTO suppliers (
			id, client_id, type, name, bin_or_iin, local_content_pct, is_active
		)
		VALUES ($1, $2, 'LEGAL', $3, $4, 75.0, TRUE)
		ON CONFLICT (id) DO NOTHING
	`, supplierID, mustUUID(t, clientID), "Test Supplier", bin)
	if err != nil {
		t.Fatalf("insert supplier: %v", err)
	}
}

func ensureContractSupplier(t *testing.T, contractID, supplierID uuid.UUID, remainingAmount float64) {
	t.Helper()

	_, err := db.Exec(ctx(), `
		INSERT INTO contract_suppliers (
			id, supplier_id, contract_number, vat_flag,
			signed_date, amount, amount_currency, currency,
			balance_at_year_end, amendment_amount,
			total_with_amendment, remaining_amount,
			is_active, created_at, updated_at, end_date
		)
		VALUES (
			$1, $2, $3, TRUE,
			CURRENT_DATE, 100000, 100000, 'KZT',
			0, 0,
			100000, $4,
			TRUE, NOW(), NOW(), CURRENT_DATE + INTERVAL '1 year'
		)
		ON CONFLICT (id) DO NOTHING
	`, contractID, supplierID, "CTR-"+contractID.String()[:8], remainingAmount)
	if err != nil {
		t.Fatalf("insert contract supplier: %v", err)
	}
}

func contractRemainingAmount(t *testing.T, contractID uuid.UUID) float64 {
	t.Helper()

	var amount float64
	if err := db.QueryRow(ctx(), `
		SELECT remaining_amount
		FROM contract_suppliers
		WHERE id = $1
	`, contractID).Scan(&amount); err != nil {
		t.Fatalf("select remaining amount: %v", err)
	}
	return amount
}

func budgetOperationCount(t *testing.T, requestID uuid.UUID, operationType string) int {
	t.Helper()

	var count int
	if err := db.QueryRow(ctx(), `
		SELECT COUNT(*)
		FROM request_budget_transactions
		WHERE request_id = $1 AND operation_type = $2
	`, requestID, operationType).Scan(&count); err != nil {
		t.Fatalf("count budget operation %s: %v", operationType, err)
	}
	return count
}

func firstChildByDZO(t *testing.T, detail *RequestDetail, dzoID uuid.UUID) uuid.UUID {
	t.Helper()

	for _, child := range detail.ChildRequests {
		if child.TargetDzoID != nil && *child.TargetDzoID == dzoID.String() {
			return uuid.MustParse(child.ID)
		}
	}
	t.Fatalf("child request for dzo %s not found", dzoID)
	return uuid.Nil
}
func mustUUID(t *testing.T, raw string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	return id
}
func floatPtr(v float64) *float64 {
	return &v
}

func costModePtr(v CostMode) *CostMode {
	return &v
}

func nullableUUIDForTest(id *uuid.UUID) interface{} {
	if id == nil {
		return nil
	}
	return *id
}

func makeDraftRequest(t *testing.T, fx *requestTestFixture, employeeIDs []uuid.UUID, dzoIDs []uuid.UUID) *RequestDetail {
	t.Helper()

	employeeStrings := make([]string, 0, len(employeeIDs))
	for _, id := range employeeIDs {
		employeeStrings = append(employeeStrings, id.String())
	}
	dzoStrings := make([]string, 0, len(dzoIDs))
	for _, id := range dzoIDs {
		dzoStrings = append(dzoStrings, id.String())
	}

	deadline := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	costMode := CostModePerEmployee
	cost := 5000.0
	resp, err := CreateAdminRequest(authCtxFor(fx.adminKC, authhandler.RoleADM, nil), &CreateAdminRequestRequest{
		TrainingEventID: fx.trainingEventID.String(),
		EmployeeIDs:     employeeStrings,
		DzoIDs:          dzoStrings,
		CostAmount:      &cost,
		CostMode:        &costMode,
		DeadlineAt:      &deadline,
	})
	if err != nil {
		t.Fatalf("create admin request: %v", err)
	}

	return &resp.Detail
}

func submitDraftRequest(t *testing.T, fx *requestTestFixture, detail *RequestDetail) *RequestDetail {
	t.Helper()

	resp, err := SubmitRequest(authCtxFor(fx.adminKC, authhandler.RoleADM, nil), toEncoreUUID(uuid.MustParse(detail.Request.ID)))
	if err != nil {
		t.Fatalf("submit request: %v", err)
	}
	return &resp.Detail
}

func createArchiveRequestForTest(t *testing.T, fx *requestTestFixture, kind RequestKind, employeeIDs []uuid.UUID, contracts []ArchiveRequestContractInput) *RequestDetail {
	t.Helper()

	rawEmployeeIDs := make([]string, 0, len(employeeIDs))
	for _, id := range employeeIDs {
		rawEmployeeIDs = append(rawEmployeeIDs, id.String())
	}

	title := "Archive request"
	resp, err := CreateArchiveRequest(authCtxFor(fx.adminKC, authhandler.RoleADM, nil), &CreateArchiveRequestRequest{
		Kind:        kind,
		Title:       &title,
		Category:    "archive_expense",
		EmployeeIDs: rawEmployeeIDs,
		Contracts:   contracts,
	})
	if err != nil {
		t.Fatalf("create archive request: %v", err)
	}

	return &resp.Detail
}

func TestCreateAdminRequest_Success(t *testing.T) {
	fx := newFixture(t)

	detail := makeDraftRequest(t, fx, []uuid.UUID{fx.empOneID}, []uuid.UUID{fx.dzoTwoID})

	if detail.Request.RequestType != RequestTypeMain {
		t.Fatalf("expected MAIN request, got %s", detail.Request.RequestType)
	}
	if detail.Request.Status != RequestStatusDraft {
		t.Fatalf("expected DRAFT status, got %s", detail.Request.Status)
	}
	if detail.Request.EmployeesCount != 1 {
		t.Fatalf("expected 1 employee, got %d", detail.Request.EmployeesCount)
	}
	if len(detail.TargetDZOs) != 1 || detail.TargetDZOs[0].ID != fx.dzoTwoID.String() {
		t.Fatalf("expected target dzo %s", fx.dzoTwoID)
	}
}

func TestSubmitRequest_SplitsByHRAndDZO(t *testing.T) {
	fx := newFixture(t)

	detail := makeDraftRequest(t, fx, []uuid.UUID{fx.empOneID}, []uuid.UUID{fx.dzoTwoID})
	submitted := submitDraftRequest(t, fx, detail)

	if submitted.Request.Status != RequestStatusInProgress {
		t.Fatalf("expected parent status IN_PROGRESS, got %s", submitted.Request.Status)
	}
	if len(submitted.ChildRequests) != 2 {
		t.Fatalf("expected 2 subrequests, got %d", len(submitted.ChildRequests))
	}

	foundDZOOne := false
	foundDZOTwo := false
	for _, child := range submitted.ChildRequests {
		if child.TargetDzoID != nil && *child.TargetDzoID == fx.dzoOneID.String() {
			foundDZOOne = true
			if child.AssignedHRID == nil || *child.AssignedHRID != fx.hrOneID.String() {
				t.Fatalf("expected hr one assigned to dzo one")
			}
			if child.EmployeesCount != 1 {
				t.Fatalf("expected 1 employee for dzo one child, got %d", child.EmployeesCount)
			}
		}
		if child.TargetDzoID != nil && *child.TargetDzoID == fx.dzoTwoID.String() {
			foundDZOTwo = true
			if child.AssignedHRID == nil || *child.AssignedHRID != fx.hrTwoID.String() {
				t.Fatalf("expected hr two assigned to dzo two")
			}
			if child.EmployeesCount != 0 {
				t.Fatalf("expected 0 employees for dzo two child, got %d", child.EmployeesCount)
			}
		}
	}

	if !foundDZOOne || !foundDZOTwo {
		t.Fatal("expected subrequests for both selected dzo groups")
	}
}

func TestListHRRequests_ReturnsOnlyAssigned(t *testing.T) {
	fx := newFixture(t)

	detail := makeDraftRequest(t, fx, []uuid.UUID{fx.empOneID, fx.empTwoID}, nil)
	_ = submitDraftRequest(t, fx, detail)

	hrOneResp, err := ListHRRequests(authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID))
	if err != nil {
		t.Fatalf("list hr requests for hr one: %v", err)
	}
	if len(hrOneResp.Items) != 1 {
		t.Fatalf("expected 1 request for hr one, got %d", len(hrOneResp.Items))
	}
	if hrOneResp.Items[0].AssignedHRID == nil || *hrOneResp.Items[0].AssignedHRID != fx.hrOneID.String() {
		t.Fatalf("unexpected assigned hr in hr one list")
	}

	hrTwoResp, err := ListHRRequests(authCtxFor(fx.hrTwoKC, authhandler.RoleHR, &fx.dzoTwoID))
	if err != nil {
		t.Fatalf("list hr requests for hr two: %v", err)
	}
	if len(hrTwoResp.Items) != 1 {
		t.Fatalf("expected 1 request for hr two, got %d", len(hrTwoResp.Items))
	}
}

func TestListHRRequests_IncludesOwnMainRequests(t *testing.T) {
	fx := newFixture(t)

	deadline := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)

	created, err := CreateHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		&CreateHRRequestRequest{
			Title:       "HR visible request",
			EmployeeIDs: []string{fx.empOneID.String()},
			DeadlineAt:  &deadline,
		},
	)
	if err != nil {
		t.Fatalf("create hr request: %v", err)
	}

	resp, err := ListHRRequests(authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID))
	if err != nil {
		t.Fatalf("list hr requests: %v", err)
	}

	found := false
	for _, item := range resp.Items {
		if item.ID != created.Detail.Request.ID {
			continue
		}
		found = true
		if item.RequestType != RequestTypeMain {
			t.Fatalf("expected MAIN request type, got %s", item.RequestType)
		}
		if item.EntityType != "HR_REQUEST" {
			t.Fatalf("expected HR_REQUEST entity type, got %s", item.EntityType)
		}
	}

	if !found {
		t.Fatal("expected HR list to include own main request")
	}
}

func TestApproveAndCancelSubrequests_UpdateParentProgress(t *testing.T) {
	fx := newFixture(t)

	detail := makeDraftRequest(t, fx, []uuid.UUID{fx.empOneID, fx.empTwoID}, nil)
	submitted := submitDraftRequest(t, fx, detail)

	var hrOneRequestID, hrTwoRequestID string
	for _, child := range submitted.ChildRequests {
		if child.TargetDzoID != nil && *child.TargetDzoID == fx.dzoOneID.String() {
			hrOneRequestID = child.ID
		}
		if child.TargetDzoID != nil && *child.TargetDzoID == fx.dzoTwoID.String() {
			hrTwoRequestID = child.ID
		}
	}

	if _, err := ApproveHRRequest(authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID), toEncoreUUID(uuid.MustParse(hrOneRequestID))); err != nil {
		t.Fatalf("approve hr one request: %v", err)
	}
	if _, err := CancelHRRequest(authCtxFor(fx.hrTwoKC, authhandler.RoleHR, &fx.dzoTwoID), toEncoreUUID(uuid.MustParse(hrTwoRequestID))); err != nil {
		t.Fatalf("cancel hr two request: %v", err)
	}

	parent, err := GetRequest(authCtxFor(fx.adminKC, authhandler.RoleADM, nil), toEncoreUUID(uuid.MustParse(detail.Request.ID)))
	if err != nil {
		t.Fatalf("get parent request: %v", err)
	}
	if parent.Detail.Request.Status != RequestStatusApproved {
		t.Fatalf("expected parent status APPROVED after mixed result, got %s", parent.Detail.Request.Status)
	}
	if parent.Detail.Request.ApprovedChildren != 1 || parent.Detail.Request.TotalChildren != 2 {
		t.Fatalf("expected progress 1/2, got %d/%d", parent.Detail.Request.ApprovedChildren, parent.Detail.Request.TotalChildren)
	}
}

func TestApproveEmptySubrequest_FailsUntilHRSetsEmployees(t *testing.T) {
	fx := newFixture(t)

	detail := makeDraftRequest(t, fx, nil, []uuid.UUID{fx.dzoThreeID})
	submitted := submitDraftRequest(t, fx, detail)
	if len(submitted.ChildRequests) != 1 {
		t.Fatalf("expected 1 child request, got %d", len(submitted.ChildRequests))
	}

	childID := uuid.MustParse(submitted.ChildRequests[0].ID)
	_, err := ApproveHRRequest(authCtxFor(fx.hrThreeKC, authhandler.RoleHR, &fx.dzoThreeID), toEncoreUUID(childID))
	if err == nil {
		t.Fatal("expected error when approving empty subrequest")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestUpdateHRRequestEmployees_ThenApprove(t *testing.T) {
	fx := newFixture(t)

	detail := makeDraftRequest(t, fx, nil, []uuid.UUID{fx.dzoTwoID})
	submitted := submitDraftRequest(t, fx, detail)
	childID := uuid.MustParse(submitted.ChildRequests[0].ID)

	updated, err := UpdateHRRequestEmployees(authCtxFor(fx.hrTwoKC, authhandler.RoleHR, &fx.dzoTwoID), toEncoreUUID(childID), &UpdateHRRequestEmployeesRequest{
		EmployeeIDs: []string{fx.empTwoID.String()},
	})
	if err != nil {
		t.Fatalf("update hr employees: %v", err)
	}
	if len(updated.Detail.Employees) != 1 {
		t.Fatalf("expected 1 employee after update, got %d", len(updated.Detail.Employees))
	}

	approved, err := ApproveHRRequest(authCtxFor(fx.hrTwoKC, authhandler.RoleHR, &fx.dzoTwoID), toEncoreUUID(childID))
	if err != nil {
		t.Fatalf("approve hr request: %v", err)
	}
	if approved.Detail.Request.Status != RequestStatusApproved {
		t.Fatalf("expected APPROVED status, got %s", approved.Detail.Request.Status)
	}
}

func TestCreateAdminRequest_RequiresHRCoverage(t *testing.T) {
	fx := newFixture(t)

	noHRDZO := uuid.New()
	ensureDZO(t, noHRDZO, fx.clientID, "No HR DZO")

	_, err := CreateAdminRequest(authCtxFor(fx.adminKC, authhandler.RoleADM, nil), &CreateAdminRequestRequest{
		TrainingEventID: fx.trainingEventID.String(),
		DzoIDs:          []string{noHRDZO.String()},
	})
	if err == nil {
		t.Fatal("expected error when DZO has no HR")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestGetHRRequest_HidesForeignSubrequest(t *testing.T) {
	fx := newFixture(t)

	detail := makeDraftRequest(t, fx, []uuid.UUID{fx.empOneID, fx.empTwoID}, nil)
	submitted := submitDraftRequest(t, fx, detail)

	childID := uuid.MustParse(submitted.ChildRequests[0].ID)
	_, err := GetHRRequest(authCtxFor(fx.hrThreeKC, authhandler.RoleHR, &fx.dzoThreeID), toEncoreUUID(childID))
	if err == nil {
		t.Fatal("expected not found for foreign hr request")
	}
	if errs.Code(err) != errs.NotFound {
		t.Fatalf("expected NotFound, got %v", errs.Code(err))
	}
}

func TestCreateArchiveRequest_Success(t *testing.T) {
	fx := newFixture(t)

	detail := createArchiveRequestForTest(t, fx, RequestKindArchived, []uuid.UUID{fx.empOneID, fx.empTwoID}, []ArchiveRequestContractInput{
		{DzoID: fx.dzoOneID.String(), FileName: "dzo-one.pdf", FileURL: "s3://contracts/dzo-one.pdf"},
		{DzoID: fx.dzoTwoID.String(), FileName: "dzo-two.pdf", FileURL: "s3://contracts/dzo-two.pdf"},
	})

	if detail.Request.RequestType != RequestTypeMain {
		t.Fatalf("expected MAIN request, got %s", detail.Request.RequestType)
	}
	if detail.Request.Kind != RequestKindArchived {
		t.Fatalf("expected ARCHIVED kind, got %s", detail.Request.Kind)
	}
	if detail.Request.Status != RequestStatusCompleted {
		t.Fatalf("expected COMPLETED status, got %s", detail.Request.Status)
	}
	if detail.Request.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
	if detail.Request.EmployeesCount != 2 {
		t.Fatalf("expected 2 employees, got %d", detail.Request.EmployeesCount)
	}
	if len(detail.DZOContracts) != 2 {
		t.Fatalf("expected 2 contracts, got %d", len(detail.DZOContracts))
	}
	if len(detail.ChildRequests) != 0 {
		t.Fatalf("expected 0 child requests, got %d", len(detail.ChildRequests))
	}
}

// budget test
func TestBudgetReserveOnSubmit(t *testing.T) {
	fx := newFixture(t)

	before := contractRemainingAmount(t, fx.contractID)

	detail := makeDraftRequest(t, fx, []uuid.UUID{fx.empOneID}, nil)
	requestID := uuid.MustParse(detail.Request.ID)

	_ = submitDraftRequest(t, fx, detail)

	after := contractRemainingAmount(t, fx.contractID)

	if after != before-5000 {
		t.Fatalf("expected remaining amount %.2f, got %.2f", before-5000, after)
	}

	if got := budgetOperationCount(t, requestID, "RESERVE"); got != 1 {
		t.Fatalf("expected 1 RESERVE transaction, got %d", got)
	}
}

func TestBudgetWriteOffOnFinalApprove(t *testing.T) {
	fx := newFixture(t)

	detail := makeDraftRequest(t, fx, []uuid.UUID{fx.empOneID}, nil)
	submitted := submitDraftRequest(t, fx, detail)

	requestID := uuid.MustParse(detail.Request.ID)
	childID := firstChildByDZO(t, submitted, fx.dzoOneID)

	if _, err := ApproveHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		toEncoreUUID(childID),
	); err != nil {
		t.Fatalf("approve hr request: %v", err)
	}
	//admin finalize
	if _, err := FinalizeAdminRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(requestID),
	); err != nil {
		t.Fatalf("finalize admin request: %v", err)
	}

	if got := budgetOperationCount(t, requestID, "WRITE_OFF"); got != 1 {
		t.Fatalf("expected 1 WRITE_OFF transaction, got %d", got)
	}

	history, err := GetRequestBudgetHistory(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(requestID),
	)
	if err != nil {
		t.Fatalf("get budget history: %v", err)
	}
	if len(history.Items) != 2 {
		t.Fatalf("expected 2 budget history items, got %d", len(history.Items))
	}
	if history.Items[0].OperationType != "RESERVE" || history.Items[1].OperationType != "WRITE_OFF" {
		t.Fatalf("unexpected budget history operations: %+v", history.Items)
	}
}

func TestBudgetReleaseOnCancelBeforeWriteOff(t *testing.T) {
	fx := newFixture(t)

	before := contractRemainingAmount(t, fx.contractID)

	detail := makeDraftRequest(t, fx, []uuid.UUID{fx.empOneID}, nil)
	submitted := submitDraftRequest(t, fx, detail)

	requestID := uuid.MustParse(submitted.Request.ID)

	afterReserve := contractRemainingAmount(t, fx.contractID)
	if afterReserve != before-5000 {
		t.Fatalf("expected remaining amount after reserve %.2f, got %.2f", before-5000, afterReserve)
	}

	if _, err := CancelAdminRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(requestID),
	); err != nil {
		t.Fatalf("cancel admin request: %v", err)
	}

	afterCancel := contractRemainingAmount(t, fx.contractID)
	if afterCancel != before {
		t.Fatalf("expected remaining amount to be released back to %.2f, got %.2f", before, afterCancel)
	}

	if got := budgetOperationCount(t, requestID, "RELEASE"); got != 1 {
		t.Fatalf("expected 1 RELEASE transaction, got %d", got)
	}
	if got := budgetOperationCount(t, requestID, "REFUND"); got != 0 {
		t.Fatalf("expected 0 REFUND transactions before write-off, got %d", got)
	}
}

func TestBudgetRefundOnCancelAfterWriteOff(t *testing.T) {
	fx := newFixture(t)

	before := contractRemainingAmount(t, fx.contractID)

	detail := makeDraftRequest(t, fx, []uuid.UUID{fx.empOneID}, nil)
	submitted := submitDraftRequest(t, fx, detail)

	requestID := uuid.MustParse(submitted.Request.ID)
	childID := firstChildByDZO(t, submitted, fx.dzoOneID)

	if _, err := ApproveHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		toEncoreUUID(childID),
	); err != nil {
		t.Fatalf("approve hr request: %v", err)
	}
	if _, err := FinalizeAdminRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(requestID),
	); err != nil {
		t.Fatalf("finalize admin request: %v", err)
	}

	afterWriteOff := contractRemainingAmount(t, fx.contractID)
	if afterWriteOff != before-5000 {
		t.Fatalf("expected remaining after write-off to stay %.2f, got %.2f", before-5000, afterWriteOff)
	}

	if _, err := CancelAdminRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(requestID),
	); err != nil {
		t.Fatalf("cancel admin request after write-off: %v", err)
	}

	afterRefund := contractRemainingAmount(t, fx.contractID)
	if afterRefund != before {
		t.Fatalf("expected remaining amount after refund %.2f, got %.2f", before, afterRefund)
	}

	if got := budgetOperationCount(t, requestID, "REFUND"); got != 1 {
		t.Fatalf("expected 1 REFUND transaction, got %d", got)
	}
}

func TestPrepareAdminRequest_Success(t *testing.T) {
	fx := newFixture(t)

	// draft -> submit
	detail := makeDraftRequest(
		t,
		fx,
		[]uuid.UUID{fx.empOneID},
		nil,
	)

	submitted := submitDraftRequest(t, fx, detail)

	childID := firstChildByDZO(t, submitted, fx.dzoOneID)

	// HR approves child
	if _, err := ApproveHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		toEncoreUUID(childID),
	); err != nil {
		t.Fatalf("approve hr request: %v", err)
	}

	// prepare
	resp, err := PrepareAdminRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(uuid.MustParse(detail.Request.ID)),
		&PrepareAdminRequestRequest{
			TrainingEventID: fx.trainingEventID.String(),
			CostAmount:      floatPtr(5000),
			CostMode:        costModePtr(CostModePerEmployee),
		},
	)
	if err != nil {
		t.Fatalf("prepare request: %v", err)
	}

	if resp.Detail.Request.TrainingEventID != fx.trainingEventID.String() {
		t.Fatalf("expected training event %s, got %s", fx.trainingEventID, resp.Detail.Request.TrainingEventID)
	}
	if resp.Detail.Request.CostAmount == nil || *resp.Detail.Request.CostAmount != 5000 {
		t.Fatalf("expected cost amount 5000, got %v", resp.Detail.Request.CostAmount)
	}
}
func TestFinalizeWithoutExplicitPrepareSucceeds(t *testing.T) {
	fx := newFixture(t)

	detail := makeDraftRequest(
		t,
		fx,
		[]uuid.UUID{fx.empOneID},
		nil,
	)

	submitted := submitDraftRequest(t, fx, detail)

	childID := firstChildByDZO(t, submitted, fx.dzoOneID)

	if _, err := ApproveHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		toEncoreUUID(childID),
	); err != nil {
		t.Fatal(err)
	}

	// Admin requests already have a training event linked at creation time,
	// so finalize does not require an explicit PrepareAdminRequest call.
	resp, err := FinalizeAdminRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(uuid.MustParse(detail.Request.ID)),
	)
	if err != nil {
		t.Fatalf("expected finalize to succeed: %v", err)
	}
	if resp.Detail.Request.Status != RequestStatusCompleted {
		t.Fatalf("expected COMPLETED, got %s", resp.Detail.Request.Status)
	}
}
func TestFinalizeMarksRequestCompleted(t *testing.T) {
	fx := newFixture(t)

	detail := makeDraftRequest(
		t,
		fx,
		[]uuid.UUID{fx.empOneID},
		nil,
	)

	submitted := submitDraftRequest(t, fx, detail)

	childID := firstChildByDZO(t, submitted, fx.dzoOneID)

	if _, err := ApproveHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		toEncoreUUID(childID),
	); err != nil {
		t.Fatal(err)
	}

	if _, err := PrepareAdminRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(uuid.MustParse(detail.Request.ID)),
		&PrepareAdminRequestRequest{
			TrainingEventID: fx.trainingEventID.String(),
			CostAmount:      floatPtr(5000),
			CostMode:        costModePtr(CostModePerEmployee),
		},
	); err != nil {
		t.Fatal(err)
	}

	resp, err := FinalizeAdminRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(uuid.MustParse(detail.Request.ID)),
	)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Detail.Request.Status != RequestStatusCompleted {
		t.Fatalf(
			"expected COMPLETED got %s",
			resp.Detail.Request.Status,
		)
	}

	if resp.Detail.Request.CompletedAt == nil {
		t.Fatal("completed_at not set")
	}
}
func TestFinalizeBlockedWhenChildRejected(t *testing.T) {
	fx := newFixture(t)

	detail := makeDraftRequest(
		t,
		fx,
		[]uuid.UUID{
			fx.empOneID,
			fx.empTwoID,
		},
		nil,
	)

	submitted := submitDraftRequest(t, fx, detail)

	child1 := firstChildByDZO(t, submitted, fx.dzoOneID)
	child2 := firstChildByDZO(t, submitted, fx.dzoTwoID)

	if _, err := ApproveHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		toEncoreUUID(child1),
	); err != nil {
		t.Fatal(err)
	}

	if _, err := CancelHRRequest(
		authCtxFor(fx.hrTwoKC, authhandler.RoleHR, &fx.dzoTwoID),
		toEncoreUUID(child2),
	); err != nil {
		t.Fatal(err)
	}

	_, err := FinalizeAdminRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(uuid.MustParse(detail.Request.ID)),
	)

	if err == nil {
		t.Fatal("expected finalize blocked with rejected child")
	}
}

func TestBudgetSubmitFailsWhenNotEnoughBudget(t *testing.T) {
	fx := newFixture(t)

	if _, err := db.Exec(ctx(), `
		UPDATE contract_suppliers
		SET remaining_amount = 100
		WHERE id = $1
	`, fx.contractID); err != nil {
		t.Fatalf("set low remaining amount: %v", err)
	}

	detail := makeDraftRequest(t, fx, []uuid.UUID{fx.empOneID}, nil)

	_, err := SubmitRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(uuid.MustParse(detail.Request.ID)),
	)
	if err == nil {
		t.Fatal("expected submit to fail when budget is insufficient")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", errs.Code(err))
	}

	if got := budgetOperationCount(t, uuid.MustParse(detail.Request.ID), "RESERVE"); got != 0 {
		t.Fatalf("expected 0 RESERVE transactions after failed submit, got %d", got)
	}
}

// CreateHRRequest creates a main request from HR to Admin.
func TestCreateHRRequest_Success(t *testing.T) {
	fx := newFixture(t)

	deadline := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)

	resp, err := CreateHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		&CreateHRRequestRequest{
			Title:       "HR requested training",
			EmployeeIDs: []string{fx.empOneID.String()},
			DeadlineAt:  &deadline,
		},
	)
	if err != nil {
		t.Fatalf("create hr request: %v", err)
	}

	detail := resp.Detail

	if detail.Request.RequestType != RequestTypeMain {
		t.Fatalf("expected MAIN request, got %s", detail.Request.RequestType)
	}
	if detail.Request.Status != RequestStatusPending {
		t.Fatalf("expected PENDING status, got %s", detail.Request.Status)
	}
	if detail.Request.EntityType != "HR_REQUEST" {
		t.Fatalf("expected HR_REQUEST entity type, got %s", detail.Request.EntityType)
	}
	if detail.Request.Title != "HR requested training" {
		t.Fatalf("unexpected title: %s", detail.Request.Title)
	}
	if detail.Request.EmployeesCount != 1 {
		t.Fatalf("expected 1 employee, got %d", detail.Request.EmployeesCount)
	}
	if len(detail.TargetDZOs) != 1 || detail.TargetDZOs[0].ID != fx.dzoOneID.String() {
		t.Fatalf("expected HR DZO as target")
	}
}

func TestGetHRRequest_AllowsHRMainRequestOwner(t *testing.T) {
	fx := newFixture(t)

	deadline := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)

	created, err := CreateHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		&CreateHRRequestRequest{
			Title:       "HR my own request",
			EmployeeIDs: []string{fx.empOneID.String()},
			DeadlineAt:  &deadline,
		},
	)
	if err != nil {
		t.Fatalf("create hr request: %v", err)
	}

	fetched, err := GetHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		toEncoreUUID(uuid.MustParse(created.Detail.Request.ID)),
	)
	if err != nil {
		t.Fatalf("fetch hr request: %v", err)
	}

	if fetched.Detail.Request.ID != created.Detail.Request.ID {
		t.Fatalf("expected fetched request id %s, got %s", created.Detail.Request.ID, fetched.Detail.Request.ID)
	}
}

func TestCreateHRRequest_RejectsEmployeeFromAnotherDZO(t *testing.T) {
	fx := newFixture(t)

	_, err := CreateHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		&CreateHRRequestRequest{
			Title:       "Wrong DZO employee",
			EmployeeIDs: []string{fx.empTwoID.String()},
		},
	)

	if err == nil {
		t.Fatal("expected error when HR selects employee from another DZO")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateHRRequest_RejectsInactiveEmployeeByDefault(t *testing.T) {
	fx := newFixture(t)

	if _, err := db.Exec(ctx(), `
		UPDATE employees
		SET is_active = FALSE
		WHERE id = $1
	`, fx.empOneID); err != nil {
		t.Fatalf("deactivate employee: %v", err)
	}

	_, err := CreateHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		&CreateHRRequestRequest{
			Title:       "Inactive employee",
			EmployeeIDs: []string{fx.empOneID.String()},
		},
	)

	if err == nil {
		t.Fatal("expected error for inactive employee")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateHRRequest_AllowsInactiveEmployeeWhenConfirmed(t *testing.T) {
	fx := newFixture(t)

	if _, err := db.Exec(ctx(), `
		UPDATE employees
		SET is_active = FALSE
		WHERE id = $1
	`, fx.empOneID); err != nil {
		t.Fatalf("deactivate employee: %v", err)
	}

	resp, err := CreateHRRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		&CreateHRRequestRequest{
			Title:                  "Inactive employee confirmed",
			EmployeeIDs:            []string{fx.empOneID.String()},
			AllowInactiveEmployees: true,
		},
	)
	if err != nil {
		t.Fatalf("create hr request with inactive employee allowed: %v", err)
	}

	if resp.Detail.Request.Status != RequestStatusPending {
		t.Fatalf("expected PENDING status, got %s", resp.Detail.Request.Status)
	}
	if resp.Detail.Request.EmployeesCount != 1 {
		t.Fatalf("expected 1 employee, got %d", resp.Detail.Request.EmployeesCount)
	}
}

func TestCreateHRRequest_OnlyHRCanCreate(t *testing.T) {
	fx := newFixture(t)

	_, err := CreateHRRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		&CreateHRRequestRequest{
			Title:       "Admin should not use HR endpoint",
			EmployeeIDs: []string{fx.empOneID.String()},
		},
	)

	if err == nil {
		t.Fatal("expected permission denied")
	}
	if errs.Code(err) != errs.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", errs.Code(err))
	}
}

func TestCreateArchiveRequest_RequiresAdmin(t *testing.T) {
	fx := newFixture(t)

	_, err := CreateArchiveRequest(authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID), &CreateArchiveRequestRequest{
		Kind:        RequestKindClosed,
		Category:    "archive_expense",
		EmployeeIDs: []string{fx.empOneID.String()},
		Contracts: []ArchiveRequestContractInput{
			{DzoID: fx.dzoOneID.String(), FileName: "dzo-one.pdf", FileURL: "s3://contracts/dzo-one.pdf"},
		},
	})
	if err == nil {
		t.Fatal("expected permission error")
	}
	if errs.Code(err) != errs.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", errs.Code(err))
	}
}

func TestCreateArchiveRequest_RequiresContractForEachDZO(t *testing.T) {
	fx := newFixture(t)

	_, err := CreateArchiveRequest(authCtxFor(fx.adminKC, authhandler.RoleADM, nil), &CreateArchiveRequestRequest{
		Kind:        RequestKindClosed,
		Category:    "archive_expense",
		EmployeeIDs: []string{fx.empOneID.String(), fx.empTwoID.String()},
		Contracts: []ArchiveRequestContractInput{
			{DzoID: fx.dzoOneID.String(), FileName: "dzo-one.pdf", FileURL: "s3://contracts/dzo-one.pdf"},
		},
	})
	if err == nil {
		t.Fatal("expected InvalidArgument error")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestSubmitArchiveRequest_Fails(t *testing.T) {
	fx := newFixture(t)

	detail := createArchiveRequestForTest(t, fx, RequestKindClosed, []uuid.UUID{fx.empOneID}, []ArchiveRequestContractInput{
		{DzoID: fx.dzoOneID.String(), FileName: "dzo-one.pdf", FileURL: "s3://contracts/dzo-one.pdf"},
	})

	_, err := SubmitRequest(authCtxFor(fx.adminKC, authhandler.RoleADM, nil), toEncoreUUID(uuid.MustParse(detail.Request.ID)))
	if err == nil {
		t.Fatal("expected invalid argument when submitting archive request")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestRecreateRejectedRequest_Success(t *testing.T) {
	fx := newFixture(t)

	// Create and reject a request
	resp, err := CreateAdminRequest(authCtxFor(fx.adminKC, authhandler.RoleADM, nil), &CreateAdminRequestRequest{
		TrainingEventID: fx.trainingEventID.String(),
		EmployeeIDs:     []string{fx.empOneID.String()},
		CostAmount:      floatPtr(50000),
		CostMode:        costModePtr(CostModePerEmployee),
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	requestID := uuid.MustParse(resp.Detail.Request.ID)

	// Manually set status to REJECTED
	_, err = db.Exec(ctx(), "UPDATE requests SET status = 'REJECTED' WHERE id = $1", requestID)
	if err != nil {
		t.Fatalf("failed to reject request: %v", err)
	}

	// Recreate the rejected request
	newResp, err := RecreateRejectedRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(requestID),
		&CreateAdminRequestRequest{
			TrainingEventID: fx.trainingEventID.String(),
			EmployeeIDs:     []string{fx.empOneID.String(), fx.empTwoID.String()},
			CostAmount:      floatPtr(75000),
			CostMode:        costModePtr(CostModePerEmployee),
		},
	)
	if err != nil {
		t.Fatalf("failed to recreate rejected request: %v", err)
	}

	if newResp.Detail.Request.Status != RequestStatusDraft {
		t.Fatalf("expected DRAFT status, got %s", newResp.Detail.Request.Status)
	}
	if newResp.Detail.Request.EmployeesCount != 2 {
		t.Fatalf("expected 2 employees, got %d", newResp.Detail.Request.EmployeesCount)
	}
	if *newResp.Detail.Request.CostAmount != 75000 {
		t.Fatalf("expected cost 75000, got %f", *newResp.Detail.Request.CostAmount)
	}
	if newResp.Detail.Request.ID == resp.Detail.Request.ID {
		t.Fatal("new request should have different ID from rejected request")
	}

	// Verify original request is blocked
	var isBlocked bool
	var replacedByID *uuid.UUID
	err = db.QueryRow(ctx(), "SELECT is_blocked, replaced_by_request_id FROM requests WHERE id = $1", requestID).Scan(&isBlocked, &replacedByID)
	if err != nil {
		t.Fatalf("failed to check blocked status: %v", err)
	}
	if !isBlocked {
		t.Fatal("original request should be blocked")
	}
	if replacedByID == nil {
		t.Fatal("replaced_by_request_id should be set")
	}
	if replacedByID.String() != newResp.Detail.Request.ID {
		t.Fatalf("replaced_by_request_id should match new request ID, got %s, expected %s", replacedByID.String(), newResp.Detail.Request.ID)
	}
}

func TestRecreateRejectedRequest_BlockedRequestCannotBeRecreated(t *testing.T) {
	fx := newFixture(t)

	// Create and reject a request
	resp, err := CreateAdminRequest(authCtxFor(fx.adminKC, authhandler.RoleADM, nil), &CreateAdminRequestRequest{
		TrainingEventID: fx.trainingEventID.String(),
		EmployeeIDs:     []string{fx.empOneID.String()},
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	requestID := uuid.MustParse(resp.Detail.Request.ID)

	// Set status to REJECTED
	_, err = db.Exec(ctx(), "UPDATE requests SET status = 'REJECTED' WHERE id = $1", requestID)
	if err != nil {
		t.Fatalf("failed to reject request: %v", err)
	}

	// Recreate once
	_, err = RecreateRejectedRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(requestID),
		&CreateAdminRequestRequest{
			TrainingEventID: fx.trainingEventID.String(),
			EmployeeIDs:     []string{fx.empOneID.String()},
		},
	)
	if err != nil {
		t.Fatalf("failed to recreate rejected request: %v", err)
	}

	// Try to recreate again (should fail because it's blocked)
	_, err = RecreateRejectedRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(requestID),
		&CreateAdminRequestRequest{
			TrainingEventID: fx.trainingEventID.String(),
			EmployeeIDs:     []string{fx.empOneID.String()},
		},
	)

	if err == nil {
		t.Fatal("expected error when recreating blocked request")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestRecreateRejectedRequest_OnlyRejectedRequests(t *testing.T) {
	fx := newFixture(t)

	resp, err := CreateAdminRequest(authCtxFor(fx.adminKC, authhandler.RoleADM, nil), &CreateAdminRequestRequest{
		TrainingEventID: fx.trainingEventID.String(),
		EmployeeIDs:     []string{fx.empOneID.String()},
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	requestID := uuid.MustParse(resp.Detail.Request.ID)

	// Try to recreate a non-rejected request
	_, err = RecreateRejectedRequest(
		authCtxFor(fx.adminKC, authhandler.RoleADM, nil),
		toEncoreUUID(requestID),
		&CreateAdminRequestRequest{
			TrainingEventID: fx.trainingEventID.String(),
			EmployeeIDs:     []string{fx.empOneID.String()},
		},
	)

	if err == nil {
		t.Fatal("expected error when recreating non-rejected request")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestRecreateRejectedRequest_RequiresAdminPermission(t *testing.T) {
	fx := newFixture(t)

	resp, err := CreateAdminRequest(authCtxFor(fx.adminKC, authhandler.RoleADM, nil), &CreateAdminRequestRequest{
		TrainingEventID: fx.trainingEventID.String(),
		EmployeeIDs:     []string{fx.empOneID.String()},
	})
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	requestID := uuid.MustParse(resp.Detail.Request.ID)

	// Set status to REJECTED
	_, err = db.Exec(ctx(), "UPDATE requests SET status = 'REJECTED' WHERE id = $1", requestID)
	if err != nil {
		t.Fatalf("failed to reject request: %v", err)
	}

	// Try to recreate as HR
	_, err = RecreateRejectedRequest(
		authCtxFor(fx.hrOneKC, authhandler.RoleHR, &fx.dzoOneID),
		toEncoreUUID(requestID),
		&CreateAdminRequestRequest{
			TrainingEventID: fx.trainingEventID.String(),
			EmployeeIDs:     []string{fx.empOneID.String()},
		},
	)

	if err == nil {
		t.Fatal("expected permission denied")
	}
	if errs.Code(err) != errs.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", errs.Code(err))
	}
}
