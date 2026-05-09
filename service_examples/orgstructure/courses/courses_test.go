// Package courses tests.
//
// This file imports encore.dev/storage/objects and encore.dev/storage/sqldb and
// cannot be run with plain go test.
// Use encore test ./orgstructure/courses/... to run these tests.
package courses

import (
	"archive/zip"
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.app/db/ent/scormcourse"
	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/google/uuid"
)

const testCompanyID = "00000000-0000-0000-0000-000000000001"

func adminCtx() context.Context {
	return auth.WithContext(
		context.Background(),
		auth.UID("courses-admin"),
		&authhandler.AuthData{
			KeycloakUserID: "courses-admin",
			Role:           authhandler.RoleADM,
			CompanyID:      testCompanyID,
		},
	)
}

func employeeCtx() context.Context {
	return auth.WithContext(
		context.Background(),
		auth.UID("courses-employee"),
		&authhandler.AuthData{
			KeycloakUserID: "courses-employee",
			Role:           authhandler.RoleEMP,
			CompanyID:      testCompanyID,
		},
	)
}

func strPtr(s string) *string {
	return &s
}

func boolPtr(v bool) *bool {
	return &v
}

func makeSCORMZip(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, body := range files {
		fileWriter, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip file %q: %v", name, err)
		}
		if _, err := fileWriter.Write([]byte(body)); err != nil {
			t.Fatalf("write zip file %q: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	return buf.Bytes()
}

func makeValidSCORMZip(t *testing.T) []byte {
	return makeSCORMZip(t, map[string]string{
		"imsmanifest.xml": `<manifest><resources><resource identifier="__5mfgNOa3Y7l_course_id_RES" type="webcontent" href="index_lms.html" adlcp:scormtype="sco"/></resources></manifest>`,
		"index_lms.html":  "<html><body>Course</body></html>",
	})
}

func makeCreateCourseRequest() *CreateCourseRequest {
	return &CreateCourseRequest{
		Title:       "Course " + uuid.NewString(),
		CategoryIDs: []uuid.UUID{uuid.New(), uuid.New()},
		Description: strPtr("Integration test course"),
		Lecturer:    strPtr("Test Lecturer"),
		ScormURL:    "scorm/uploads/" + uuid.NewString() + ".zip",
	}
}

func mustCreateCourse(t *testing.T) *Course {
	t.Helper()

	resp, err := CreateCourse(adminCtx(), makeCreateCourseRequest())
	if err != nil {
		t.Fatalf("CreateCourse setup failed: %v", err)
	}
	if resp.Course == nil {
		t.Fatal("CreateCourse setup returned nil course")
	}

	return resp.Course
}

func mustGetCourseRow(t *testing.T, id uuid.UUID) *ent.ScormCourse {
	t.Helper()

	row, err := Client.ScormCourse.Query().Where(scormcourse.ID(id)).Only(context.Background())
	if err != nil {
		t.Fatalf("query course row: %v", err)
	}

	return row
}

func requireErrCode(t *testing.T, err error, code errs.ErrCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %v error, got nil", code)
	}
	if errs.Code(err) != code {
		t.Fatalf("expected %v, got %v: %v", code, errs.Code(err), err)
	}
}

func TestUploadSCORM_Success(t *testing.T) {
	req := &UploadSCORMRequest{
		FileName: "My Course Package.zip",
		FileData: makeValidSCORMZip(t),
	}

	resp, err := UploadSCORM(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.FileName != req.FileName {
		t.Errorf("expected file_name %q, got %q", req.FileName, resp.FileName)
	}
	if resp.FileSize != len(req.FileData) {
		t.Errorf("expected file_size %d, got %d", len(req.FileData), resp.FileSize)
	}
	if strings.TrimSpace(resp.ScormURL) == "" {
		t.Fatal("expected non-empty scorm_url")
	}
	if !resp.IsValid {
		t.Error("expected upload to be valid")
	}
}

func TestValidateSCORM_RequiresSCOWebContentResource(t *testing.T) {
	_, err := validateSCORM(makeSCORMZip(t, map[string]string{
		"imsmanifest.xml": `<manifest><resources><resource href="index_lms.html"/></resources></manifest>`,
		"index_lms.html":  "<html><body>Course</body></html>",
	}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "invalid SCORM: entry point not found" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUploadSCORM_InvalidExtension(t *testing.T) {
	_, err := UploadSCORM(adminCtx(), &UploadSCORMRequest{
		FileName: "course.pdf",
		FileData: []byte("abc"),
	})
	requireErrCode(t, err, errs.InvalidArgument)
}

func TestUploadSCORM_EmptyFileData(t *testing.T) {
	_, err := UploadSCORM(adminCtx(), &UploadSCORMRequest{
		FileName: "course.zip",
		FileData: nil,
	})
	requireErrCode(t, err, errs.InvalidArgument)
}

func TestUploadSCORM_EmployeeDenied(t *testing.T) {
	_, err := UploadSCORM(employeeCtx(), &UploadSCORMRequest{
		FileName: "course.zip",
		FileData: []byte("abc"),
	})
	requireErrCode(t, err, errs.PermissionDenied)
}

func TestUploadCourseImage_SuccessExtensions(t *testing.T) {
	for _, fileName := range []string{"preview.png", "preview.jpg", "preview.jpeg", "preview.webp"} {
		t.Run(fileName, func(t *testing.T) {
			req := &UploadCourseImageRequest{
				FileName: fileName,
				FileData: bytes.Repeat([]byte("image-data"), 4),
			}

			resp, err := UploadCourseImage(adminCtx(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.FileName != req.FileName {
				t.Errorf("expected file_name %q, got %q", req.FileName, resp.FileName)
			}
			if resp.FileSize != len(req.FileData) {
				t.Errorf("expected file_size %d, got %d", len(req.FileData), resp.FileSize)
			}
			if strings.TrimSpace(resp.ImageURL) == "" {
				t.Fatal("expected non-empty image_url")
			}
			if resp.Message != "Course image uploaded successfully" {
				t.Errorf("unexpected message %q", resp.Message)
			}
		})
	}
}

func TestUploadCourseImage_InvalidExtension(t *testing.T) {
	_, err := UploadCourseImage(adminCtx(), &UploadCourseImageRequest{
		FileName: "preview.gif",
		FileData: []byte("abc"),
	})
	requireErrCode(t, err, errs.InvalidArgument)
}

func TestUploadCourseImage_EmptyFileData(t *testing.T) {
	_, err := UploadCourseImage(adminCtx(), &UploadCourseImageRequest{
		FileName: "preview.png",
		FileData: nil,
	})
	requireErrCode(t, err, errs.InvalidArgument)
}

func TestCreateCourse_Success(t *testing.T) {
	req := makeCreateCourseRequest()

	resp, err := CreateCourse(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Course == nil {
		t.Fatal("expected course, got nil")
	}
	if resp.Course.Title != req.Title {
		t.Errorf("expected title %q, got %q", req.Title, resp.Course.Title)
	}
	if !reflect.DeepEqual(resp.Course.CategoryIDs, req.CategoryIDs) {
		t.Errorf("expected category_ids %v, got %v", req.CategoryIDs, resp.Course.CategoryIDs)
	}
	if resp.Course.Description == nil || *resp.Course.Description != *req.Description {
		t.Errorf("expected description %q, got %v", *req.Description, resp.Course.Description)
	}
	if resp.Course.Lecturer == nil || *resp.Course.Lecturer != *req.Lecturer {
		t.Errorf("expected lecturer %q, got %v", *req.Lecturer, resp.Course.Lecturer)
	}
	if resp.Course.ScormURL != req.ScormURL {
		t.Errorf("expected scorm_url %q, got %q", req.ScormURL, resp.Course.ScormURL)
	}
	if !resp.Course.IsActive {
		t.Error("expected new course to be active")
	}

	row := mustGetCourseRow(t, resp.Course.ID)
	if row.ClientID.String() != testCompanyID {
		t.Errorf("expected client_id %q, got %q", testCompanyID, row.ClientID.String())
	}
	if !reflect.DeepEqual(row.CategoryIds, req.CategoryIDs) {
		t.Errorf("expected stored category_ids %v, got %v", req.CategoryIDs, row.CategoryIds)
	}
}

func TestCreateCourse_OptionalFieldsWork(t *testing.T) {
	req := makeCreateCourseRequest()
	req.Description = strPtr("Optional description")
	req.Lecturer = strPtr("Optional lecturer")
	req.ImageURL = strPtr("course-images/preview.png")

	resp, err := CreateCourse(adminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Course.Description == nil || *resp.Course.Description != *req.Description {
		t.Fatalf("expected description %q, got %v", *req.Description, resp.Course.Description)
	}
	if resp.Course.Lecturer == nil || *resp.Course.Lecturer != *req.Lecturer {
		t.Fatalf("expected lecturer %q, got %v", *req.Lecturer, resp.Course.Lecturer)
	}
	if resp.Course.ImageURL == nil || *resp.Course.ImageURL != *req.ImageURL {
		t.Fatalf("expected image_url %q, got %v", *req.ImageURL, resp.Course.ImageURL)
	}
}

func TestCreateCourse_EmptyTitle(t *testing.T) {
	req := makeCreateCourseRequest()
	req.Title = "   "

	_, err := CreateCourse(adminCtx(), req)
	requireErrCode(t, err, errs.InvalidArgument)
}

func TestCreateCourse_EmptyCategoryIDs(t *testing.T) {
	req := makeCreateCourseRequest()
	req.CategoryIDs = nil

	_, err := CreateCourse(adminCtx(), req)
	requireErrCode(t, err, errs.InvalidArgument)
}

func TestCreateCourse_EmptyScormURL(t *testing.T) {
	req := makeCreateCourseRequest()
	req.ScormURL = "   "

	_, err := CreateCourse(adminCtx(), req)
	requireErrCode(t, err, errs.InvalidArgument)
}

func TestCreateCourse_EmployeeDenied(t *testing.T) {
	_, err := CreateCourse(employeeCtx(), makeCreateCourseRequest())
	requireErrCode(t, err, errs.PermissionDenied)
}

func TestListCourses_SuccessReturnsCreatedCourse(t *testing.T) {
	created := mustCreateCourse(t)

	resp, err := ListCourses(adminCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Courses == nil {
		t.Fatal("expected non-nil courses slice")
	}

	found := false
	for _, course := range resp.Courses {
		if course.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected created course %s in list", created.ID)
	}
}

func TestListCourses_ReturnsEmptyArrayInsteadOfNilWhenNoMatches(t *testing.T) {
	courses, err := listCourses(context.Background(), uuid.New(), authhandler.RoleADM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if courses == nil {
		t.Fatal("expected empty non-nil slice")
	}
	if len(courses) != 0 {
		t.Fatalf("expected no courses for random company, got %d", len(courses))
	}
}

func TestListCourses_AdminSeesActiveAndInactive(t *testing.T) {
	active := mustCreateCourse(t)
	inactive := mustCreateCourse(t)

	if err := DeleteCourse(adminCtx(), inactive.ID.String()); err != nil {
		t.Fatalf("DeleteCourse setup failed: %v", err)
	}

	resp, err := ListCourses(adminCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundActive := false
	foundInactive := false
	for _, course := range resp.Courses {
		if course.ID == active.ID {
			foundActive = true
		}
		if course.ID == inactive.ID {
			foundInactive = true
		}
	}

	if !foundActive {
		t.Error("expected active course in admin list")
	}
	if !foundInactive {
		t.Error("expected inactive course in admin list")
	}
}

func TestListCourses_EmployeeSeesOnlyActiveCourses(t *testing.T) {
	active := mustCreateCourse(t)
	inactive := mustCreateCourse(t)

	if err := DeleteCourse(adminCtx(), inactive.ID.String()); err != nil {
		t.Fatalf("DeleteCourse setup failed: %v", err)
	}

	resp, err := ListCourses(employeeCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundActive := false
	foundInactive := false
	for _, course := range resp.Courses {
		if !course.IsActive {
			t.Fatalf("employee list should not include inactive course %s", course.ID)
		}
		if course.ID == active.ID {
			foundActive = true
		}
		if course.ID == inactive.ID {
			foundInactive = true
		}
	}

	if !foundActive {
		t.Error("expected active course in employee list")
	}
	if foundInactive {
		t.Error("did not expect inactive course in employee list")
	}
}

func TestGetCourse_Success(t *testing.T) {
	created := mustCreateCourse(t)

	resp, err := GetCourse(adminCtx(), created.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Course == nil {
		t.Fatal("expected course, got nil")
	}
	if resp.Course.ID != created.ID {
		t.Errorf("expected id %q, got %q", created.ID, resp.Course.ID)
	}
	if !reflect.DeepEqual(resp.Course.CategoryIDs, created.CategoryIDs) {
		t.Errorf("expected category_ids %v, got %v", created.CategoryIDs, resp.Course.CategoryIDs)
	}
}

func TestGetCourse_InvalidUUID(t *testing.T) {
	_, err := GetCourse(adminCtx(), "not-a-uuid")
	requireErrCode(t, err, errs.InvalidArgument)
}

func TestGetCourse_UnknownUUID(t *testing.T) {
	_, err := GetCourse(adminCtx(), uuid.NewString())
	requireErrCode(t, err, errs.NotFound)
}

func TestGetCourse_EmployeeCannotGetInactiveCourse(t *testing.T) {
	created := mustCreateCourse(t)

	if err := DeleteCourse(adminCtx(), created.ID.String()); err != nil {
		t.Fatalf("DeleteCourse setup failed: %v", err)
	}

	_, err := GetCourse(employeeCtx(), created.ID.String())
	requireErrCode(t, err, errs.NotFound)
}

func TestUpdateCourse_Success(t *testing.T) {
	created := mustCreateCourse(t)

	title := "Updated " + uuid.NewString()
	description := "Updated description"
	lecturer := "Updated lecturer"
	scormURL := "scorm/uploads/" + uuid.NewString() + ".zip"
	categoryIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	imageURL := "course-images/updated.webp"
	isActive := false

	resp, err := UpdateCourse(adminCtx(), created.ID.String(), &UpdateCourseRequest{
		Title:       &title,
		CategoryIDs: &categoryIDs,
		Description: &description,
		Lecturer:    &lecturer,
		ScormURL:    &scormURL,
		ImageURL:    &imageURL,
		IsActive:    &isActive,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Course.Title != title {
		t.Errorf("expected title %q, got %q", title, resp.Course.Title)
	}
	if !reflect.DeepEqual(resp.Course.CategoryIDs, categoryIDs) {
		t.Errorf("expected category_ids %v, got %v", categoryIDs, resp.Course.CategoryIDs)
	}
	if resp.Course.Description == nil || *resp.Course.Description != description {
		t.Errorf("expected description %q, got %v", description, resp.Course.Description)
	}
	if resp.Course.Lecturer == nil || *resp.Course.Lecturer != lecturer {
		t.Errorf("expected lecturer %q, got %v", lecturer, resp.Course.Lecturer)
	}
	if resp.Course.ScormURL != scormURL {
		t.Errorf("expected scorm_url %q, got %q", scormURL, resp.Course.ScormURL)
	}
	if resp.Course.ImageURL == nil || *resp.Course.ImageURL != imageURL {
		t.Errorf("expected image_url %q, got %v", imageURL, resp.Course.ImageURL)
	}
	if resp.Course.IsActive != isActive {
		t.Errorf("expected is_active %v, got %v", isActive, resp.Course.IsActive)
	}

	row := mustGetCourseRow(t, created.ID)
	if !reflect.DeepEqual(row.CategoryIds, categoryIDs) {
		t.Errorf("expected stored category_ids %v, got %v", categoryIDs, row.CategoryIds)
	}
	if row.ImageURL == nil || *row.ImageURL != imageURL {
		t.Errorf("expected stored image_url %q, got %v", imageURL, row.ImageURL)
	}
}

func TestUpdateCourse_EmptyTitle(t *testing.T) {
	created := mustCreateCourse(t)
	empty := "   "

	_, err := UpdateCourse(adminCtx(), created.ID.String(), &UpdateCourseRequest{Title: &empty})
	requireErrCode(t, err, errs.InvalidArgument)
}

func TestUpdateCourse_EmptyCategoryIDs(t *testing.T) {
	created := mustCreateCourse(t)
	categoryIDs := []uuid.UUID{}

	_, err := UpdateCourse(adminCtx(), created.ID.String(), &UpdateCourseRequest{
		CategoryIDs: &categoryIDs,
	})
	requireErrCode(t, err, errs.InvalidArgument)
}

func TestUpdateCourse_EmptyScormURL(t *testing.T) {
	created := mustCreateCourse(t)
	empty := "   "

	_, err := UpdateCourse(adminCtx(), created.ID.String(), &UpdateCourseRequest{
		ScormURL: &empty,
	})
	requireErrCode(t, err, errs.InvalidArgument)
}

func TestUpdateCourse_UnknownUUID(t *testing.T) {
	title := "Unknown"

	_, err := UpdateCourse(adminCtx(), uuid.NewString(), &UpdateCourseRequest{Title: &title})
	requireErrCode(t, err, errs.NotFound)
}

func TestUpdateCourse_EmployeeDenied(t *testing.T) {
	created := mustCreateCourse(t)
	title := "Employee Attempt"

	_, err := UpdateCourse(employeeCtx(), created.ID.String(), &UpdateCourseRequest{
		Title: &title,
	})
	requireErrCode(t, err, errs.PermissionDenied)
}

func TestDeleteCourse_SuccessSoftDeletes(t *testing.T) {
	created := mustCreateCourse(t)

	if err := DeleteCourse(adminCtx(), created.ID.String()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	row := mustGetCourseRow(t, created.ID)
	if row.IsActive {
		t.Error("expected is_active=false after soft delete")
	}
}

func TestDeleteCourse_UnknownUUID(t *testing.T) {
	err := DeleteCourse(adminCtx(), uuid.NewString())
	requireErrCode(t, err, errs.NotFound)
}

func TestDeleteCourse_EmployeeDenied(t *testing.T) {
	created := mustCreateCourse(t)

	err := DeleteCourse(employeeCtx(), created.ID.String())
	requireErrCode(t, err, errs.PermissionDenied)
}

func TestDeleteCourse_EmployeeNoLongerSeesDeletedCourse(t *testing.T) {
	created := mustCreateCourse(t)

	if err := DeleteCourse(adminCtx(), created.ID.String()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	listResp, err := ListCourses(employeeCtx())
	if err != nil {
		t.Fatalf("ListCourses failed: %v", err)
	}
	for _, course := range listResp.Courses {
		if course.ID == created.ID {
			t.Fatalf("employee should not see deleted course %s in list", created.ID)
		}
	}

	_, err = GetCourse(employeeCtx(), created.ID.String())
	requireErrCode(t, err, errs.NotFound)
}
