package courses

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
	"time"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/storage/objects"
	"encore.dev/storage/sqldb"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.app/db/ent/scormcourse"
	//"encore.app/db/ent/scormprogress"
)

// DATABASE

var (
	db           = sqldb.Named("lms")
	Client       = newEntClient()
	scormBucket  = objects.NewBucket("scorm-files", objects.BucketConfig{})
	publicAssets = objects.NewBucket("public-assets", objects.BucketConfig{
		Public: true,
	})
)

func newEntClient() *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, db.Stdlib())
	return ent.NewClient(ent.Driver(drv))
}

// ENDPOINTS

//encore:api auth method=POST path=/courses/upload-scorm
func UploadSCORM(ctx context.Context, req *UploadSCORMRequest) (*UploadSCORMResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}
	if err := requireRole(ad, authhandler.RoleSA, authhandler.RoleADM); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg("request body is required").
			Err()
	}

	if err := validateUploadSCORMRequest(req.FileName, req.FileData); err != nil {
		return nil, err
	}
	entryPoint, err := validateSCORM(req.FileData)
	if err != nil {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg(err.Error()).
			Err()
	}
	if strings.TrimSpace(entryPoint) == "" {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg("invalid SCORM: entry point not found").
			Err()
	}
	scormURL, err := uploadSCORMToStorage(ctx, req.FileName, req.FileData)
	if err != nil {
		return nil, err
	}

	return &UploadSCORMResponse{
		FileName: req.FileName,
		FileSize: len(req.FileData),
		ScormURL: scormURL,
		IsValid:  true,
		Message:  "SCORM package uploaded successfully",
	}, nil
}

//encore:api auth method=POST path=/courses/upload-image
func UploadCourseImage(ctx context.Context, req *UploadCourseImageRequest) (*UploadCourseImageResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}
	if err := requireRole(ad, authhandler.RoleSA, authhandler.RoleADM); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg("request body is required").
			Err()
	}

	if err := validateUploadCourseImageRequest(req.FileName, req.FileData); err != nil {
		return nil, err
	}

	imageURL, err := uploadCourseImageToStorage(ctx, req.FileName, req.FileData)
	if err != nil {
		return nil, err
	}

	return &UploadCourseImageResponse{
		FileName: req.FileName,
		FileSize: len(req.FileData),
		ImageURL: imageURL,
		Message:  "Course image uploaded successfully",
	}, nil
}

//encore:api auth method=POST path=/courses
func CreateCourse(ctx context.Context, req *CreateCourseRequest) (*GetCourseResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}
	if err := requireRole(ad, authhandler.RoleSA, authhandler.RoleADM); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg("request body is required").
			Err()
	}
	clientUID, err := uuid.Parse(ad.CompanyID)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid company_id in token").Err()
	}
	course, err := insertCourse(ctx, clientUID, req)
	if err != nil {
		return nil, err
	}

	return &GetCourseResponse{
		Course: course,
	}, nil
}

//encore:api auth method=GET path=/courses
func ListCourses(ctx context.Context) (*ListCoursesResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}
	clientUID, err := uuid.Parse(ad.CompanyID)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid company_id in token").Err()
	}
	courses, err := listCourses(ctx, clientUID, ad.Role)
	if err != nil {
		return nil, err
	}

	return &ListCoursesResponse{
		Courses: courses,
	}, nil
}

//encore:api auth method=GET path=/courses/:id
func GetCourse(ctx context.Context, id string) (*GetCourseResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}
	clientUID, err := uuid.Parse(ad.CompanyID)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid company_id in token").Err()
	}
	course, err := getCourseByID(ctx, clientUID, ad.Role, id)
	if err != nil {
		return nil, err
	}

	return &GetCourseResponse{
		Course: course,
	}, nil
}

//encore:api auth method=PATCH path=/courses/:id
func UpdateCourse(ctx context.Context, id string, req *UpdateCourseRequest) (*GetCourseResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}
	if err := requireRole(ad, authhandler.RoleSA, authhandler.RoleADM); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg("request body is required").
			Err()
	}
	clientUID, err := uuid.Parse(ad.CompanyID)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid company_id in token").Err()
	}
	course, err := updateCourse(ctx, clientUID, ad.Role, id, req)
	if err != nil {
		return nil, err
	}

	return &GetCourseResponse{
		Course: course,
	}, nil
}

//encore:api auth method=DELETE path=/courses/:id
func DeleteCourse(ctx context.Context, id string) error {
	ad, err := getAuthData()
	if err != nil {
		return err
	}
	if err := requireRole(ad, authhandler.RoleSA, authhandler.RoleADM); err != nil {
		return err
	}
	clientUID, err := uuid.Parse(ad.CompanyID)
	if err != nil {
		return errs.B().Code(errs.InvalidArgument).Msg("invalid company_id in token").Err()
	}
	if err := softDeleteCourse(ctx, clientUID, ad.Role, id); err != nil {
		return err
	}

	return nil
}

// INTERNAL

// func sanitizeFileName(name string) string {
// 	name = filepath.Base(name)
// 	name = strings.ReplaceAll(name, " ", "_")
// 	return name
// }

func validateUploadSCORMRequest(fileName string, fileData []byte) error {
	if strings.TrimSpace(fileName) == "" {
		return errs.B().
			Code(errs.InvalidArgument).
			Msg("file_name is required").
			Err()
	}

	if len(fileData) == 0 {
		return errs.B().
			Code(errs.InvalidArgument).
			Msg("file_data is required").
			Err()
	}

	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != ".zip" {
		return errs.B().
			Code(errs.InvalidArgument).
			Msg("only .zip files are allowed").
			Err()
	}

	return nil
}
func validateSCORM(fileData []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(fileData), int64(len(fileData)))
	if err != nil {
		return "", fmt.Errorf("invalid SCORM: invalid zip archive")
	}

	files := make(map[string]bool, len(reader.File))
	var manifestFile *zip.File
	for _, file := range reader.File {
		normalizedName := normalizeSCORMPath(file.Name)
		files[normalizedName] = true
		if strings.EqualFold(path.Base(normalizedName), "imsmanifest.xml") {
			manifestFile = file
		}
	}
	if manifestFile == nil {
		return "", fmt.Errorf("invalid SCORM: imsmanifest.xml not found")
	}

	href, err := readSCORMEntryPoint(manifestFile)
	if err != nil {
		return "", err
	}

	entryPoint := normalizeSCORMPath(href)
	manifestDir := path.Dir(normalizeSCORMPath(manifestFile.Name))
	if manifestDir != "." && manifestDir != "/" {
		entryPoint = normalizeSCORMPath(path.Join(manifestDir, entryPoint))
	}
	if !files[entryPoint] {
		return "", fmt.Errorf("invalid SCORM: entry point not found")
	}

	return entryPoint, nil
}
func readSCORMEntryPoint(manifestFile *zip.File) (string, error) {
	rc, err := manifestFile.Open()
	if err != nil {
		return "", fmt.Errorf("invalid SCORM: failed to read imsmanifest.xml")
	}
	defer rc.Close()

	decoder := xml.NewDecoder(rc)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return "", fmt.Errorf("invalid SCORM: entry point not found")
		}
		if err != nil {
			return "", fmt.Errorf("invalid SCORM: failed to parse imsmanifest.xml")
		}

		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "resource" {
			continue
		}
		var href string
		var isWebContent bool
		var isSCO bool
		for _, attr := range start.Attr {
			value := strings.TrimSpace(attr.Value)
			switch attr.Name.Local {
			case "href":
				href = value
			case "type":
				isWebContent = strings.EqualFold(value, "webcontent")
			case "scormtype":
				isSCO = strings.EqualFold(value, "sco")
			}
		}
		if href != "" && isWebContent && isSCO {
			return href, nil
		}
	}
}
func normalizeSCORMPath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	p = strings.TrimLeft(p, "/")
	for strings.HasPrefix(p, "./") {
		p = strings.TrimPrefix(p, "./")
	}
	if p == "" {
		return ""
	}
	return path.Clean(p)
}
func validateUploadCourseImageRequest(fileName string, fileData []byte) error {
	if strings.TrimSpace(fileName) == "" {
		return errs.B().
			Code(errs.InvalidArgument).
			Msg("file_name is required").
			Err()
	}

	if len(fileData) == 0 {
		return errs.B().
			Code(errs.InvalidArgument).
			Msg("file_data is required").
			Err()
	}

	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return nil
	default:
		return errs.B().
			Code(errs.InvalidArgument).
			Msg("only .jpg, .jpeg, .png, and .webp files are allowed").
			Err()
	}
}

func uploadSCORMToStorage(ctx context.Context, fileName string, fileData []byte) (string, error) {
	ext := filepath.Ext(fileName)
	objectKey := fmt.Sprintf(
		"scorm/uploads/%s%s",
		uuid.NewString(),
		ext,
	)

	writer := scormBucket.Upload(ctx, objectKey)

	_, err := writer.Write(fileData)
	if err != nil {
		writer.Abort(err)
		return "", errs.B().
			Code(errs.Internal).
			Msg("failed to upload SCORM package").
			Cause(err).
			Err()
	}

	if err := writer.Close(); err != nil {
		writer.Abort(err)
		return "", errs.B().
			Code(errs.Internal).
			Msg("failed to finalize SCORM upload").
			Cause(err).
			Err()
	}
	url, err := generateSignedURL(ctx, objectKey)
	if err != nil {
		return "", err
	}
	return url, nil
}

func uploadCourseImageToStorage(ctx context.Context, fileName string, fileData []byte) (string, error) {
	ext := filepath.Ext(fileName)
	objectKey := fmt.Sprintf(
		"course-images/%s%s",
		uuid.NewString(),
		ext,
	)

	writer := publicAssets.Upload(ctx, objectKey)

	_, err := writer.Write(fileData)
	if err != nil {
		writer.Abort(err)
		return "", errs.B().
			Code(errs.Internal).
			Msg("failed to upload course image").
			Cause(err).
			Err()
	}

	if err := writer.Close(); err != nil {
		writer.Abort(err)
		return "", errs.B().
			Code(errs.Internal).
			Msg("failed to finalize course image upload").
			Cause(err).
			Err()
	}
	url, err := generatePublicURL(objectKey)
	if err != nil {
		return "", err
	}
	return url, nil
}

func generateSignedURL(ctx context.Context, key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", errs.B().Code(errs.NotFound).Msg("object key not found").Err()
	}

	signedURL, err := scormBucket.SignedDownloadURL(ctx, key, objects.WithTTL(240*time.Minute))
	if err != nil {
		return "", errs.B().
			Code(errs.Internal).
			Msg("failed to generate signed URL").
			Cause(err).
			Err()
	}

	return signedURL.URL, nil
}
func generatePublicURL(key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", errs.B().Code(errs.NotFound).Msg("object key not found").Err()
	}
	publicURL := publicAssets.PublicURL(key)
	return publicURL.String(), nil
}
func insertCourse(ctx context.Context, clientUID uuid.UUID, req *CreateCourseRequest) (*Course, error) {
	if strings.TrimSpace(req.Title) == "" {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg("title is required").
			Err()
	}

	if len(req.CategoryIDs) == 0 {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg("category_ids is required").
			Err()
	}

	if strings.TrimSpace(req.ScormURL) == "" {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg("scorm_url is required").
			Err()
	}

	builder := Client.ScormCourse.
		Create().
		SetClientID(clientUID).
		SetTitle(strings.TrimSpace(req.Title)).
		SetCategoryIds(req.CategoryIDs).
		SetScormURL(strings.TrimSpace(req.ScormURL))

	if req.Description != nil {
		builder = builder.SetDescription(strings.TrimSpace(*req.Description))
	}

	if req.Lecturer != nil {
		builder = builder.SetLecturer(strings.TrimSpace(*req.Lecturer))
	}

	if req.ImageURL != nil {
		builder = builder.SetImageURL(strings.TrimSpace(*req.ImageURL))
	}

	row, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, errs.B().
				Code(errs.AlreadyExists).
				Msg("course already exists").
				Err()
		}

		return nil, errs.B().
			Code(errs.Internal).
			Msg("failed to create course").
			Cause(err).
			Err()
	}

	return entToCourse(row), nil
}

func listCourses(ctx context.Context, clientUID uuid.UUID, role authhandler.UserRole) ([]*Course, error) {
	q := Client.ScormCourse.
		Query()

	if role == authhandler.RoleEMP {
		q = q.Where(scormcourse.IsActive(true))
	}
	if role == authhandler.RoleADM || role == authhandler.RoleHR || role == authhandler.RoleEMP {
		q = q.Where(scormcourse.ClientID(clientUID))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, errs.B().
			Code(errs.Internal).
			Msg("failed to list courses").
			Cause(err).
			Err()
	}

	result := make([]*Course, 0, len(rows))
	for _, row := range rows {
		course := entToCourse(row)
		result = append(result, course)
	}

	return result, nil
}

func getCourseByID(ctx context.Context, clientUID uuid.UUID, role authhandler.UserRole, id string) (*Course, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid id format").Err()
	}
	q := Client.ScormCourse.
		Query().
		Where(
			scormcourse.ID(uid),
		)

	if role == authhandler.RoleEMP {
		q = q.Where(scormcourse.IsActive(true))
	}
	if role == authhandler.RoleADM || role == authhandler.RoleHR || role == authhandler.RoleEMP {
		q = q.Where(scormcourse.ClientID(clientUID))
	}
	row, err := q.Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().
				Code(errs.NotFound).
				Msg("course not found").
				Err()
		}

		return nil, errs.B().
			Code(errs.Internal).
			Msg("failed to get course").
			Cause(err).
			Err()
	}

	return entToCourse(row), nil
}

func updateCourse(ctx context.Context, clientUID uuid.UUID, role authhandler.UserRole, id string, req *UpdateCourseRequest) (*Course, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid id format").Err()
	}
	if req.Title != nil && strings.TrimSpace(*req.Title) == "" {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg("title cannot be empty").
			Err()
	}

	if req.CategoryIDs != nil && len(*req.CategoryIDs) == 0 {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg("category_ids cannot be empty").
			Err()
	}

	if req.ScormURL != nil && strings.TrimSpace(*req.ScormURL) == "" {
		return nil, errs.B().
			Code(errs.InvalidArgument).
			Msg("scorm_url cannot be empty").
			Err()
	}

	q := Client.ScormCourse.
		Query().
		Where(scormcourse.ID(uid))
	if role == authhandler.RoleADM || role == authhandler.RoleHR {
		q = q.Where(scormcourse.ClientID(clientUID))
	}
	row, err := q.Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().
				Code(errs.NotFound).
				Msg("course not found").
				Err()
		}

		return nil, errs.B().
			Code(errs.Internal).
			Msg("failed to find course").
			Cause(err).
			Err()
	}

	builder := Client.ScormCourse.UpdateOneID(row.ID)

	if req.Title != nil {
		builder = builder.SetTitle(strings.TrimSpace(*req.Title))
	}
	if req.CategoryIDs != nil {
		builder = builder.SetCategoryIds(*req.CategoryIDs)
	}
	if req.Description != nil {
		builder = builder.SetDescription(strings.TrimSpace(*req.Description))
	}
	if req.Lecturer != nil {
		builder = builder.SetLecturer(strings.TrimSpace(*req.Lecturer))
	}
	if req.ScormURL != nil {
		builder = builder.SetScormURL(strings.TrimSpace(*req.ScormURL))
	}

	if (req.ImageURL != nil) && (*req.ImageURL != "") {
		builder = builder.SetImageURL(strings.TrimSpace(*req.ImageURL))
	} else {
		builder = builder.SetImageURL(strings.TrimSpace(*row.ImageURL))
	}
	if req.IsActive != nil {
		builder = builder.SetIsActive(*req.IsActive)
	}

	updatedRow, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, errs.B().
				Code(errs.AlreadyExists).
				Msg("course update conflicts with existing data").
				Err()
		}

		return nil, errs.B().
			Code(errs.Internal).
			Msg("failed to update course").
			Cause(err).
			Err()
	}

	return entToCourse(updatedRow), nil
}

func softDeleteCourse(ctx context.Context, clientUID uuid.UUID, role authhandler.UserRole, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return errs.B().Code(errs.InvalidArgument).Msg("invalid id format").Err()
	}
	q := Client.ScormCourse.
		Query().
		Where(scormcourse.ID(uid))
	if role == authhandler.RoleADM || role == authhandler.RoleHR {
		q = q.Where(scormcourse.ClientID(clientUID))
	}
	row, err := q.Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return errs.B().
				Code(errs.NotFound).
				Msg("course not found").
				Err()
		}

		return errs.B().
			Code(errs.Internal).
			Msg("failed to find course").
			Cause(err).
			Err()
	}

	_, err = Client.ScormCourse.
		UpdateOneID(row.ID).
		SetIsActive(false).
		Save(ctx)
	if err != nil {
		return errs.B().
			Code(errs.Internal).
			Msg("failed to delete course").
			Cause(err).
			Err()
	}

	return nil
}

// HELPERS

func entToCourse(row *ent.ScormCourse) *Course {
	if row == nil {
		return nil
	}

	return &Course{
		ID:          row.ID,
		ClientID:    row.ClientID,
		Title:       row.Title,
		CategoryIDs: row.CategoryIds,
		Description: row.Description,
		Lecturer:    row.Lecturer,
		ScormURL:    row.ScormURL,
		ImageURL:    row.ImageURL,
		IsActive:    row.IsActive,
	}
}

func getAuthData() (*authhandler.AuthData, error) {
	ad, ok := auth.Data().(*authhandler.AuthData)
	if !ok {
		return nil, errs.B().Code(errs.Unauthenticated).Msg("not authenticated").Err()
	}
	return ad, nil
}

func requireRole(ad *authhandler.AuthData, allowed ...authhandler.UserRole) error {
	for _, r := range allowed {
		if ad.Role == r {
			return nil
		}
	}
	return errs.B().Code(errs.PermissionDenied).Msg("insufficient permissions").Err()
}
