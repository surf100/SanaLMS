// Package scorm_progress tests.
//
// This file imports encore.dev/storage/sqldb and cannot be run with plain go test.
// Use encore test ./orgstructure/scorm_progress/... to run these tests.
package scorm_progress

import (
	"context"
	"testing"
	"time"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.app/db/ent/company"
	"encore.app/db/ent/scormprogress"
	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/google/uuid"
)

const testCompanyID = "00000000-0000-0000-0000-000000000001"

func adminCtx() context.Context {
	return auth.WithContext(context.Background(), auth.UID("scorm-admin"), &authhandler.AuthData{
		KeycloakUserID: "scorm-admin",
		Email:          "scorm-admin@test.local",
		Role:           authhandler.RoleADM,
		CompanyID:      testCompanyID,
	})
}

func employeeCtx(keycloakID, email string) context.Context {
	return auth.WithContext(context.Background(), auth.UID(keycloakID), &authhandler.AuthData{
		KeycloakUserID: keycloakID,
		Email:          email,
		Role:           authhandler.RoleEMP,
		CompanyID:      testCompanyID,
	})
}

func mustCreateCompanyAndDZO(t *testing.T) (uuid.UUID, uuid.UUID) {
	t.Helper()

	companyID := uuid.MustParse(testCompanyID)
	_, err := Client.Company.
		Query().
		Where(company.IDEQ(companyID)).
		Only(context.Background())
	if err != nil {
		if !ent.IsNotFound(err) {
			t.Fatalf("query company: %v", err)
		}
		if _, err := Client.Company.
			Create().
			SetID(companyID).
			SetName("SCORM Test Company").
			Save(context.Background()); err != nil {
			t.Fatalf("create company: %v", err)
		}
	}

	dzoID := uuid.New()
	if _, err := Client.DzoOrganization.
		Create().
		SetID(dzoID).
		SetClientID(companyID).
		SetName("SCORM Test DZO " + dzoID.String()).
		Save(context.Background()); err != nil {
		t.Fatalf("create dzo: %v", err)
	}

	return companyID, dzoID
}

func mustCreateEmployee(t *testing.T, companyID, dzoID uuid.UUID, email string) *ent.Employee {
	t.Helper()

	row, err := Client.Employee.
		Create().
		SetClientID(companyID).
		SetDzoID(dzoID).
		SetFullName("Employee " + uuid.NewString()).
		SetEmail(email).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create employee: %v", err)
	}
	return row
}

func mustCreateLinkedEmployee(t *testing.T, companyID, dzoID uuid.UUID, keycloakID, email string) *ent.Employee {
	t.Helper()

	userRow, err := Client.User.
		Create().
		SetEmail(email).
		SetKeycloakUserID(keycloakID).
		SetRole("EMP").
		SetClientID(companyID).
		SetDzoID(dzoID).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	emp, err := Client.Employee.
		Create().
		SetClientID(companyID).
		SetDzoID(dzoID).
		SetFullName("Employee " + uuid.NewString()).
		SetEmail(email).
		SetUserID(userRow.ID).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create linked employee: %v", err)
	}

	return emp
}

func mustCreateCourse(t *testing.T, companyID uuid.UUID) *ent.ScormCourse {
	t.Helper()

	row, err := Client.ScormCourse.
		Create().
		SetClientID(companyID).
		SetTitle("Course " + uuid.NewString()).
		SetCategoryIds([]uuid.UUID{uuid.New()}).
		SetScormURL("scorm/uploads/" + uuid.NewString() + ".zip").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create course: %v", err)
	}
	return row
}

func mustCreateProgress(t *testing.T, courseID, employeeID uuid.UUID, status scormprogress.Status) *ent.ScormProgress {
	t.Helper()

	builder := Client.ScormProgress.
		Create().
		SetCourseID(courseID).
		SetEmployeeID(employeeID).
		SetStatus(status)

	row, err := builder.Save(context.Background())
	if err != nil {
		t.Fatalf("create progress: %v", err)
	}
	return row
}

func mustGetProgress(t *testing.T, progressID uuid.UUID) *ent.ScormProgress {
	t.Helper()

	row, err := Client.ScormProgress.
		Query().
		Where(scormprogress.IDEQ(progressID)).
		Only(context.Background())
	if err != nil {
		t.Fatalf("get progress: %v", err)
	}
	return row
}

func TestAssignCourseEmployees_WhenSomeEmployeeAssigned(t *testing.T) {
	companyID, dzoID := mustCreateCompanyAndDZO(t)
	course := mustCreateCourse(t, companyID)
	emp1 := mustCreateEmployee(t, companyID, dzoID, uuid.NewString()+"@test.local")
	emp2 := mustCreateEmployee(t, companyID, dzoID, uuid.NewString()+"@test.local")
	_, err := AssignCourseEmployees(adminCtx(), course.ID.String(), &AssignCourseEmployeesRequest{
		EmployeeIDs: []uuid.UUID{emp1.ID, emp2.ID},
		Reassign:    false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, err := AssignCourseEmployees(adminCtx(), course.ID.String(), &AssignCourseEmployeesRequest{
		EmployeeIDs: []uuid.UUID{emp1.ID, emp2.ID},
		Reassign:    false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.RequiresReassign {
		t.Fatal("requies RequiresReassign to be true")
	}
}
func TestAssignCourseEmployees_SuccessWhenNoEmployeesAlreadyAssigned(t *testing.T) {
	companyID, dzoID := mustCreateCompanyAndDZO(t)
	course := mustCreateCourse(t, companyID)
	emp1 := mustCreateEmployee(t, companyID, dzoID, uuid.NewString()+"@test.local")
	emp2 := mustCreateEmployee(t, companyID, dzoID, uuid.NewString()+"@test.local")

	resp, err := AssignCourseEmployees(adminCtx(), course.ID.String(), &AssignCourseEmployeesRequest{
		EmployeeIDs: []uuid.UUID{emp1.ID, emp2.ID},
		Reassign:    false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.RequiresReassign {
		t.Fatal("expected requires_reassign=false")
	}
	if resp.AssignedCount != 2 {
		t.Fatalf("expected assigned_count=2, got %d", resp.AssignedCount)
	}

	count, err := Client.ScormProgress.
		Query().
		Where(
			scormprogress.CourseIDEQ(course.ID),
			scormprogress.EmployeeIDIn(emp1.ID, emp2.ID),
		).
		Count(context.Background())
	if err != nil {
		t.Fatalf("count progress rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 progress rows, got %d", count)
	}
}

func TestAssignCourseEmployees_ReassignFalseReturnsAlreadyAssignedAndMakesNoChanges(t *testing.T) {
	companyID, dzoID := mustCreateCompanyAndDZO(t)
	course := mustCreateCourse(t, companyID)
	emp1 := mustCreateEmployee(t, companyID, dzoID, uuid.NewString()+"@test.local")
	emp2 := mustCreateEmployee(t, companyID, dzoID, uuid.NewString()+"@test.local")

	existing, err := Client.ScormProgress.
		Create().
		SetCourseID(course.ID).
		SetEmployeeID(emp1.ID).
		SetStatus(scormprogress.StatusIN_PROGRESS).
		SetScore(77).
		SetSuspendData("keep-me").
		Save(context.Background())
	if err != nil {
		t.Fatalf("setup progress: %v", err)
	}

	resp, err := AssignCourseEmployees(adminCtx(), course.ID.String(), &AssignCourseEmployeesRequest{
		EmployeeIDs: []uuid.UUID{emp1.ID, emp2.ID},
		Reassign:    false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.RequiresReassign {
		t.Fatal("expected requires_reassign=true")
	}
	if resp.AssignedCount != 0 {
		t.Fatalf("expected assigned_count=0, got %d", resp.AssignedCount)
	}
	if resp.AlreadyAssignedCount != 1 {
		t.Fatalf("expected already_assigned_count=1, got %d", resp.AlreadyAssignedCount)
	}

	rows, err := Client.ScormProgress.
		Query().
		Where(scormprogress.CourseIDEQ(course.ID)).
		All(context.Background())
	if err != nil {
		t.Fatalf("query progress rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 progress row, got %d", len(rows))
	}
	if rows[0].ID != existing.ID {
		t.Fatalf("expected existing row to remain unchanged")
	}
	if rows[0].Status != scormprogress.StatusIN_PROGRESS {
		t.Fatalf("expected existing status to remain IN_PROGRESS, got %s", rows[0].Status)
	}
}

func TestAssignCourseEmployees_ReassignTrueResetsExistingAndCreatesNewRows(t *testing.T) {
	companyID, dzoID := mustCreateCompanyAndDZO(t)
	course := mustCreateCourse(t, companyID)
	emp1 := mustCreateEmployee(t, companyID, dzoID, uuid.NewString()+"@test.local")
	emp2 := mustCreateEmployee(t, companyID, dzoID, uuid.NewString()+"@test.local")
	completedAt := time.Now().UTC().Add(-2 * time.Hour)

	existing, err := Client.ScormProgress.
		Create().
		SetCourseID(course.ID).
		SetEmployeeID(emp1.ID).
		SetStatus(scormprogress.StatusCOMPLETED).
		SetScore(98).
		SetCompletedAt(completedAt).
		SetSuspendData("old-state").
		Save(context.Background())
	if err != nil {
		t.Fatalf("setup progress: %v", err)
	}

	resp, err := AssignCourseEmployees(adminCtx(), course.ID.String(), &AssignCourseEmployeesRequest{
		EmployeeIDs: []uuid.UUID{emp1.ID, emp2.ID},
		Reassign:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.RequiresReassign {
		t.Fatal("expected requires_reassign=false")
	}
	if resp.ReassignedCount != 1 {
		t.Fatalf("expected reassigned_count=1, got %d", resp.ReassignedCount)
	}
	if resp.AssignedCount != 1 {
		t.Fatalf("expected assigned_count=1, got %d", resp.AssignedCount)
	}

	resetRow := mustGetProgress(t, existing.ID)
	if resetRow.Status != scormprogress.StatusNOT_STARTED {
		t.Fatalf("expected status NOT_STARTED, got %s", resetRow.Status)
	}
	if resetRow.Score != nil {
		t.Fatalf("expected score to be cleared, got %v", *resetRow.Score)
	}
	if resetRow.CompletedAt != nil {
		t.Fatalf("expected completed_at to be cleared")
	}
	if resetRow.SuspendData != nil {
		t.Fatalf("expected suspend_data to be cleared")
	}

	newCount, err := Client.ScormProgress.
		Query().
		Where(
			scormprogress.CourseIDEQ(course.ID),
			scormprogress.EmployeeIDEQ(emp2.ID),
		).
		Count(context.Background())
	if err != nil {
		t.Fatalf("count new progress: %v", err)
	}
	if newCount != 1 {
		t.Fatalf("expected 1 new progress row, got %d", newCount)
	}
}

func TestUpdateCourseProgress_Success(t *testing.T) {
	companyID, dzoID := mustCreateCompanyAndDZO(t)
	keycloakID := "kc-" + uuid.NewString()
	email := uuid.NewString() + "@test.local"
	course := mustCreateCourse(t, companyID)
	emp := mustCreateLinkedEmployee(t, companyID, dzoID, keycloakID, email)
	progress := mustCreateProgress(t, course.ID, emp.ID, scormprogress.StatusNOT_STARTED)
	score := 90
	status := "COMPLETED"
	suspendData := "bookmark"

	resp, err := UpdateCourseProgress(employeeCtx(keycloakID, email), progress.ID.String(), &UpdateCourseProgressRequest{
		Status:      &status,
		Score:       &score,
		SuspendData: &suspendData,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Progress.Status != "COMPLETED" {
		t.Fatalf("expected COMPLETED, got %s", resp.Progress.Status)
	}
	if resp.Progress.Score == nil || *resp.Progress.Score != 90 {
		t.Fatalf("expected score=90, got %v", resp.Progress.Score)
	}
	if resp.Progress.CompletedAt == nil {
		t.Fatal("expected completed_at to be auto-set")
	}
}

func TestUpdateCourseProgress_InvalidStatus(t *testing.T) {
	companyID, dzoID := mustCreateCompanyAndDZO(t)
	course := mustCreateCourse(t, companyID)
	emp := mustCreateEmployee(t, companyID, dzoID, uuid.NewString()+"@test.local")
	progress := mustCreateProgress(t, course.ID, emp.ID, scormprogress.StatusNOT_STARTED)
	invalidStatus := "BROKEN"

	_, err := UpdateCourseProgress(adminCtx(), progress.ID.String(), &UpdateCourseProgressRequest{
		Status: &invalidStatus,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestGetMyCourseProgress_Success(t *testing.T) {
	companyID, dzoID := mustCreateCompanyAndDZO(t)
	keycloakID := "kc-" + uuid.NewString()
	email := uuid.NewString() + "@test.local"
	emp := mustCreateLinkedEmployee(t, companyID, dzoID, keycloakID, email)
	course1 := mustCreateCourse(t, companyID)
	course2 := mustCreateCourse(t, companyID)
	progress := mustCreateProgress(t, course1.ID, emp.ID, scormprogress.StatusNOT_STARTED)
	mustCreateProgress(t, course2.ID, emp.ID, scormprogress.StatusIN_PROGRESS)

	resp, err := GetMyCourseProgress(employeeCtx(keycloakID, email), course1.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Progress.ID != progress.ID {
		t.Fatalf("expected progress id %s, got %s", progress.ID, resp.Progress.ID)
	}
	if resp.Progress.CourseID != course1.ID {
		t.Fatalf("expected course id %s, got %s", course1.ID, resp.Progress.CourseID)
	}
	if resp.Progress.Status != scormprogress.StatusNOT_STARTED.String() {
		t.Fatalf("expected status %s, got %s", scormprogress.StatusNOT_STARTED, resp.Progress.Status)
	}
}

func TestListEmployeeCourses_Success(t *testing.T) {
	companyID, dzoID := mustCreateCompanyAndDZO(t)
	emp := mustCreateEmployee(t, companyID, dzoID, uuid.NewString()+"@test.local")
	course := mustCreateCourse(t, companyID)

	score := 55
	suspendData := "resume"
	completedAt := time.Now().UTC()
	_, err := Client.ScormProgress.
		Create().
		SetCourseID(course.ID).
		SetEmployeeID(emp.ID).
		SetStatus(scormprogress.StatusIN_PROGRESS).
		SetScore(score).
		SetSuspendData(suspendData).
		SetCompletedAt(completedAt).
		Save(context.Background())
	if err != nil {
		t.Fatalf("setup progress: %v", err)
	}

	resp, err := ListEmployeeCourses(adminCtx(), emp.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Courses) != 1 {
		t.Fatalf("expected 1 course, got %d", len(resp.Courses))
	}
	if resp.Courses[0].ID != course.ID {
		t.Fatalf("expected course id %s, got %s", course.ID, resp.Courses[0].ID)
	}
	if resp.Courses[0].Progress.Status != "IN_PROGRESS" {
		t.Fatalf("expected IN_PROGRESS, got %s", resp.Courses[0].Progress.Status)
	}
}
