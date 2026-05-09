package categories

import (
	"context"
	"strings"

	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"encore.app/db/ent"
	"encore.app/db/ent/category"
)

var (
	db     = sqldb.Named("lms")
	Client = newEntClient()
)

func newEntClient() *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, db.Stdlib())
	return ent.NewClient(ent.Driver(drv))
}

//////////////////////////////////////////////////////////
// CREATE
//////////////////////////////////////////////////////////

//encore:api auth method=POST path=/categories
func CreateCategory(ctx context.Context, req *CreateCategoryRequest) (*GetCategoryResponse, error) {
	if req == nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("request body is required").Err()
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("name is required").Err()
	}

	builder := Client.Category.
		Create().
		SetName(name)

	if req.Description != nil {
		desc := strings.TrimSpace(*req.Description)
		builder.SetDescription(desc)
	}

	row, err := builder.Save(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to create category").Cause(err).Err()
	}

	return &GetCategoryResponse{
		Category: *entToCategory(row),
	}, nil
}

//////////////////////////////////////////////////////////
// LIST
//////////////////////////////////////////////////////////

//encore:api auth method=GET path=/categories
func ListCategories(ctx context.Context) (*ListCategoriesResponse, error) {
	rows, err := Client.Category.
		Query().
		Order(ent.Asc(category.FieldName)).
		All(ctx)

	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to list categories").Cause(err).Err()
	}

	result := make([]Category, 0, len(rows))
	for _, r := range rows {
		result = append(result, *entToCategory(r))
	}

	return &ListCategoriesResponse{Categories: result}, nil
}

//////////////////////////////////////////////////////////
// GET BY ID
//////////////////////////////////////////////////////////

//encore:api auth method=GET path=/categories/:id
func GetCategory(ctx context.Context, id string) (*GetCategoryResponse, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid id").Err()
	}

	row, err := Client.Category.
		Query().
		Where(category.IDEQ(uid)).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("category not found").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to get category").Cause(err).Err()
	}

	return &GetCategoryResponse{
		Category: *entToCategory(row),
	}, nil
}

//////////////////////////////////////////////////////////
// UPDATE
//////////////////////////////////////////////////////////

//encore:api auth method=PATCH path=/categories/:id
func UpdateCategory(ctx context.Context, id string, req *UpdateCategoryRequest) (*GetCategoryResponse, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid id").Err()
	}

	builder := Client.Category.UpdateOneID(uid)

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("name cannot be empty").Err()
		}
		builder.SetName(name)
	}

	if req.Description != nil {
		desc := strings.TrimSpace(*req.Description)
		builder.SetDescription(desc)
	}

	row, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("category not found").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to update category").Cause(err).Err()
	}

	return &GetCategoryResponse{
		Category: *entToCategory(row),
	}, nil
}

//////////////////////////////////////////////////////////
// DELETE
//////////////////////////////////////////////////////////

//encore:api auth method=DELETE path=/categories/:id
func DeleteCategory(ctx context.Context, id string) (*DeleteCategoryResponse, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid id").Err()
	}

	err = Client.Category.
		DeleteOneID(uid).
		Exec(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("category not found").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to delete category").Cause(err).Err()
	}

	return &DeleteCategoryResponse{
		Message: "category deleted",
	}, nil
}

//////////////////////////////////////////////////////////
// MAPPER
//////////////////////////////////////////////////////////

func entToCategory(e *ent.Category) *Category {
	return &Category{
		ID:          e.ID.String(),
		Name:        e.Name,
		Description: e.Description,
	}
}