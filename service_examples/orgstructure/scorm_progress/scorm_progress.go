package scorm_progress

import (
	"context"
	"strings"
	"time"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.app/db/ent/employee"
	"encore.app/db/ent/scormcourse"
	"encore.app/db/ent/scormprogress"
	"encore.app/db/ent/user"
)

var (
	db     = sqldb.Named("lms")
	Client = newEntClient()
)

func newEntClient() *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, db.Stdlib())
	return ent.NewClient(ent.Driver(drv))
}

//encore:api auth method=POST path=/course-progress/courses/:course_id/assign-employees
func AssignCourseEmployees(ctx context.Context, course_id string, req *AssignCourseEmployeesRequest) (*AssignCourseEmployeesResponse, error) {
	courseID, err := uuid.Parse(course_id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid course_id format").Err()
	}
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}
	if err := requireRole(ad, authhandler.RoleSA, authhandler.RoleADM, authhandler.RoleHR); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("request body is required").Err()
	}

	employeeIDs := uniqueUUIDs(req.EmployeeIDs)
	if len(employeeIDs) == 0 {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("employee_ids cannot be empty").Err()
	}

	course, err := getCourseForCaller(ctx, ad, courseID)
	if err != nil {
		return nil, err
	}

	employees, err := getEmployeesByIDs(ctx, employeeIDs)
	if err != nil {
		return nil, err
	}
	if len(employees) != len(employeeIDs) {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("one or more employees not found").Err()
	}
	for _, emp := range employees {
		if err := ensureEmployeeInScope(ad, emp); err != nil {
			return nil, err
		}
		if ad.Role != authhandler.RoleSA && emp.ClientID != course.ClientID {
			return nil, errs.B().Code(errs.NotFound).Msg("employee not found").Err()
		}
	}

	existingRows, err := Client.ScormProgress.
		Query().
		Where(
			scormprogress.CourseIDEQ(courseID),
			scormprogress.EmployeeIDIn(employeeIDs...),
		).
		Order(scormprogress.ByEmployeeID()).
		All(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to query existing course assignments").Cause(err).Err()
	}

	alreadyAssignedIDs := make([]uuid.UUID, 0, len(existingRows))
	existingByEmployeeID := make(map[uuid.UUID]*ent.ScormProgress, len(existingRows))
	for _, row := range existingRows {
		existingByEmployeeID[row.EmployeeID] = row
		alreadyAssignedIDs = append(alreadyAssignedIDs, row.EmployeeID)
	}

	if !req.Reassign && len(alreadyAssignedIDs) > 0 {
		return &AssignCourseEmployeesResponse{
			RequiresReassign:           true,
			AssignedCount:              0,
			ReassignedCount:            0,
			AlreadyAssignedCount:       len(alreadyAssignedIDs),
			AssignedEmployeeIDs:        []uuid.UUID{},
			ReassignedEmployeeIDs:      []uuid.UUID{},
			AlreadyAssignedEmployeeIDs: alreadyAssignedIDs,
			Message:                    "some employees already have this course assigned",
		}, nil
	}

	tx, err := Client.Tx(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to begin transaction").Cause(err).Err()
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	assignedIDs := make([]uuid.UUID, 0, len(employeeIDs))
	reassignedIDs := make([]uuid.UUID, 0, len(alreadyAssignedIDs))

	if req.Reassign && len(alreadyAssignedIDs) > 0 {
		if _, err = tx.ScormProgress.
			Update().
			Where(
				scormprogress.CourseIDEQ(courseID),
				scormprogress.EmployeeIDIn(alreadyAssignedIDs...),
			).
			SetStatus(scormprogress.StatusNOT_STARTED).
			ClearScore().
			ClearCompletedAt().
			ClearSuspendData().
			Save(ctx); err != nil {
			return nil, errs.B().Code(errs.Internal).Msg("failed to reassign course progress").Cause(err).Err()
		}
		reassignedIDs = append(reassignedIDs, alreadyAssignedIDs...)
	}

	builders := make([]*ent.ScormProgressCreate, 0, len(employeeIDs))
	for _, employeeID := range employeeIDs {
		if _, ok := existingByEmployeeID[employeeID]; ok {
			continue
		}
		builders = append(builders, tx.ScormProgress.
			Create().
			SetCourseID(courseID).
			SetEmployeeID(employeeID).
			SetStatus(scormprogress.StatusNOT_STARTED))
		assignedIDs = append(assignedIDs, employeeID)
	}

	if len(builders) > 0 {
		if _, err = tx.ScormProgress.CreateBulk(builders...).Save(ctx); err != nil {
			return nil, errs.B().Code(errs.Internal).Msg("failed to assign course to employees").Cause(err).Err()
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to commit course assignment").Cause(err).Err()
	}

	return &AssignCourseEmployeesResponse{
		RequiresReassign:           false,
		AssignedCount:              len(assignedIDs),
		ReassignedCount:            len(reassignedIDs),
		AlreadyAssignedCount:       len(alreadyAssignedIDs),
		AssignedEmployeeIDs:        assignedIDs,
		ReassignedEmployeeIDs:      reassignedIDs,
		AlreadyAssignedEmployeeIDs: alreadyAssignedIDs,
		Message:                    "course employees processed successfully",
	}, nil
}

//encore:api auth method=PATCH path=/course-progress/:progress_id
func UpdateCourseProgress(ctx context.Context, progress_id string, req *UpdateCourseProgressRequest) (*GetCourseProgressResponse, error) {
	progressID, err := uuid.Parse(progress_id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid progress_id format").Err()
	}
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("request body is required").Err()
	}

	_, _, err = getAccessibleProgress(ctx, ad, progressID)
	if err != nil {
		return nil, err
	}

	builder := Client.ScormProgress.UpdateOneID(progressID)
	completedAtHandled := false

	if req.Status != nil {
		status, err := parseStatus(*req.Status)
		if err != nil {
			return nil, err
		}
		builder = builder.SetStatus(status)
		if status == scormprogress.StatusCOMPLETED && req.CompletedAt == nil {
			now := time.Now().UTC()
			builder = builder.SetCompletedAt(now)
			completedAtHandled = true
		}
	}
	if req.Score != nil {
		builder = builder.SetScore(*req.Score)
	}
	if req.SuspendData != nil {
		builder = builder.SetSuspendData(*req.SuspendData)
	}
	if req.CompletedAt != nil && !completedAtHandled {
		builder = builder.SetCompletedAt(*req.CompletedAt)
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("course progress not found").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to update course progress").Cause(err).Err()
	}

	return &GetCourseProgressResponse{Progress: entToCourseProgress(updated)}, nil
}

//encore:api auth method=GET path=/me/course-progress/:course_id
func GetMyCourseProgress(ctx context.Context, course_id string) (*GetMyCourseProgressResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}
	if err := requireRole(ad, authhandler.RoleEMP); err != nil {
		return nil, err
	}

	courseID, err := uuid.Parse(course_id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid course_id format").Err()
	}
	employeeID, err := resolveCurrentEmployeeID(ctx, ad)
	if err != nil {
		return nil, err
	}

	row, err := Client.ScormProgress.
		Query().
		Where(scormprogress.EmployeeIDEQ(employeeID)).
		Where(scormprogress.CourseIDEQ(courseID)).
		Only(ctx)
	progress := CourseProgress{}
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("course is not assigned").Err()
		} else {
			return nil, errs.B().Code(errs.Internal).Msg("failed to get course").Cause(err).Err()
		}
	} else {
		progress = entToCourseProgress(row)
	}

	return &GetMyCourseProgressResponse{Progress: progress}, nil
}

//encore:api auth method=GET path=/employees/:employee_id/courses
func ListEmployeeCourses(ctx context.Context, employee_id string) (*ListEmployeeCoursesResponse, error) {
	employeeID, err := uuid.Parse(employee_id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid employee_id format").Err()
	}
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}
	if err := requireRole(ad, authhandler.RoleSA, authhandler.RoleADM, authhandler.RoleHR); err != nil {
		return nil, err
	}

	emp, err := getEmployeeByID(ctx, employeeID)
	if err != nil {
		return nil, err
	}
	if err := ensureEmployeeInScope(ad, emp); err != nil {
		return nil, err
	}

	rows, err := Client.ScormProgress.
		Query().
		Where(scormprogress.EmployeeIDEQ(employeeID)).
		WithProgress().
		Order(scormprogress.ByID()).
		All(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to list employee courses").Cause(err).Err()
	}

	courses := make([]CourseWithProgress, 0, len(rows))
	for _, row := range rows {
		course := row.Edges.Progress
		if course == nil {
			continue
		}
		courses = append(courses, CourseWithProgress{
			ID:          course.ID,
			ClientID:    course.ClientID,
			Title:       course.Title,
			CategoryIDs: course.CategoryIds,
			Description: course.Description,
			Lecturer:    course.Lecturer,
			ScormURL:    course.ScormURL,
			IsActive:    course.IsActive,
			Progress:    entToCourseProgress(row),
		})
	}

	return &ListEmployeeCoursesResponse{Courses: courses}, nil
}

func getAuthData() (*authhandler.AuthData, error) {
	ad, ok := auth.Data().(*authhandler.AuthData)
	if !ok {
		return nil, errs.B().Code(errs.Unauthenticated).Msg("not authenticated").Err()
	}
	return ad, nil
}

func requireRole(ad *authhandler.AuthData, allowed ...authhandler.UserRole) error {
	for _, role := range allowed {
		if ad.Role == role {
			return nil
		}
	}
	return errs.B().Code(errs.PermissionDenied).Msg("insufficient permissions").Err()
}

func parseStatus(raw string) (scormprogress.Status, error) {
	status := scormprogress.Status(strings.TrimSpace(raw))
	if err := scormprogress.StatusValidator(status); err != nil {
		return "", errs.B().Code(errs.InvalidArgument).Msg("invalid status").Err()
	}
	return status, nil
}

func getCourseForCaller(ctx context.Context, ad *authhandler.AuthData, courseID uuid.UUID) (*ent.ScormCourse, error) {
	row, err := Client.ScormCourse.
		Query().
		Where(scormcourse.ID(courseID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("course not found").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to get course").Cause(err).Err()
	}

	if ad.Role == authhandler.RoleSA {
		return row, nil
	}

	companyID, err := parseCompanyID(ad)
	if err != nil {
		return nil, err
	}
	if row.ClientID != companyID {
		return nil, errs.B().Code(errs.NotFound).Msg("course not found").Err()
	}

	return row, nil
}

func getEmployeeByID(ctx context.Context, employeeID uuid.UUID) (*ent.Employee, error) {
	row, err := Client.Employee.
		Query().
		Where(
			employee.IDEQ(employeeID),
			employee.IsDeletedEQ(false),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("employee not found").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to get employee").Cause(err).Err()
	}
	return row, nil
}

func getEmployeesByIDs(ctx context.Context, employeeIDs []uuid.UUID) ([]*ent.Employee, error) {
	rows, err := Client.Employee.
		Query().
		Where(
			employee.IDIn(employeeIDs...),
			employee.IsDeletedEQ(false),
		).
		All(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to query employees").Cause(err).Err()
	}
	return rows, nil
}

//	func createNewCourseProgress(ctx context.Context, courseID uuid.UUID, employeeID uuid.UUID) (CourseProgress, error) {
//		builder := Client.ScormProgress.Create().
//			SetCourseID(courseID).
//			SetEmployeeID(employeeID).
//			SetStatus(scormprogress.StatusNOT_STARTED)
//		row, err := builder.Save(ctx)
//		if err != nil {
//			return CourseProgress{}, errs.B().Code(errs.Internal).Msg("failed to assign this course to employee").Cause(err).Err()
//		}
//		return entToCourseProgress(row), nil
//	}
func ensureEmployeeInScope(ad *authhandler.AuthData, emp *ent.Employee) error {
	switch ad.Role {
	case authhandler.RoleSA:
		return nil
	case authhandler.RoleADM:
		companyID, err := parseCompanyID(ad)
		if err != nil {
			return err
		}
		if emp.ClientID != companyID {
			return errs.B().Code(errs.NotFound).Msg("employee not found").Err()
		}
		return nil
	case authhandler.RoleHR:
		companyID, err := parseCompanyID(ad)
		if err != nil {
			return err
		}
		if emp.ClientID != companyID {
			return errs.B().Code(errs.NotFound).Msg("employee not found").Err()
		}
		if strings.TrimSpace(ad.DzoID) != "" && emp.DzoID.String() != ad.DzoID {
			return errs.B().Code(errs.NotFound).Msg("employee not found").Err()
		}
		return nil
	default:
		return errs.B().Code(errs.PermissionDenied).Msg("insufficient permissions").Err()
	}
}

func getAccessibleProgress(ctx context.Context, ad *authhandler.AuthData, progressID uuid.UUID) (*ent.ScormProgress, *ent.Employee, error) {
	row, err := Client.ScormProgress.
		Query().
		Where(scormprogress.IDEQ(progressID)).
		WithProgress().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil, errs.B().Code(errs.NotFound).Msg("course progress not found").Err()
		}
		return nil, nil, errs.B().Code(errs.Internal).Msg("failed to get course progress").Cause(err).Err()
	}

	emp, err := getEmployeeByID(ctx, row.EmployeeID)
	if err != nil {
		if errs.Code(err) == errs.NotFound {
			return nil, nil, errs.B().Code(errs.NotFound).Msg("course progress not found").Err()
		}
		return nil, nil, err
	}

	switch ad.Role {
	case authhandler.RoleSA:
		return row, emp, nil
	case authhandler.RoleADM, authhandler.RoleHR:
		if err := ensureEmployeeInScope(ad, emp); err != nil {
			return nil, nil, errs.B().Code(errs.NotFound).Msg("course progress not found").Err()
		}
		return row, emp, nil
	case authhandler.RoleEMP:
		currentEmployeeID, err := resolveCurrentEmployeeID(ctx, ad)
		if err != nil {
			return nil, nil, err
		}
		if row.EmployeeID != currentEmployeeID {
			return nil, nil, errs.B().Code(errs.NotFound).Msg("course progress not found").Err()
		}
		return row, emp, nil
	default:
		return nil, nil, errs.B().Code(errs.PermissionDenied).Msg("insufficient permissions").Err()
	}
}

func resolveCurrentEmployeeID(ctx context.Context, ad *authhandler.AuthData) (uuid.UUID, error) {
	if strings.TrimSpace(ad.KeycloakUserID) != "" {
		userRow, err := Client.User.
			Query().
			Where(user.KeycloakUserIDEQ(ad.KeycloakUserID)).
			Only(ctx)
		if err == nil {
			employeeRow, err := Client.Employee.
				Query().
				Where(
					employee.UserIDEQ(userRow.ID),
					employee.IsDeletedEQ(false),
				).
				Only(ctx)
			if err == nil {
				return employeeRow.ID, nil
			}
			if err != nil && !ent.IsNotFound(err) {
				return uuid.Nil, errs.B().Code(errs.Internal).Msg("failed to resolve current employee").Cause(err).Err()
			}
		} else if !ent.IsNotFound(err) {
			return uuid.Nil, errs.B().Code(errs.Internal).Msg("failed to resolve current user").Cause(err).Err()
		}
	}
	return uuid.Nil, errs.B().Code(errs.Internal).Msg("failed to resolve current user").Err()
}

func parseCompanyID(ad *authhandler.AuthData) (uuid.UUID, error) {
	companyID, err := uuid.Parse(ad.CompanyID)
	if err != nil {
		return uuid.Nil, errs.B().Code(errs.InvalidArgument).Msg("invalid company_id in token").Err()
	}
	return companyID, nil
}

func uniqueUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	result := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func entToCourseProgress(row *ent.ScormProgress) CourseProgress {
	return CourseProgress{
		ID:          row.ID,
		CourseID:    row.CourseID,
		EmployeeID:  row.EmployeeID,
		Status:      row.Status.String(),
		Score:       row.Score,
		CompletedAt: row.CompletedAt,
		SuspendData: row.SuspendData,
	}
}
