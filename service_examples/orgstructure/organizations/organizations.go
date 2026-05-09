package organizations

import (
	"context"
	"strings"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.app/db/ent/organization"
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

// CreateOrg creates a new organization. SA only.
//
//encore:api auth method=POST path=/organizations
func CreateOrg(ctx context.Context, req *CreateOrgRequest) (*GetOrgResponse, error) {
	if err := requireRole(authhandler.RoleSA); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("name is required").Err()
	}
	if strings.TrimSpace(req.Code) == "" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("code is required").Err()
	}
	if !req.Type.IsValid() {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid organization type").Err()
	}

	org, err := insertOrg(ctx, req)
	if err != nil {
		return nil, err
	}

	return &GetOrgResponse{Organization: *org}, nil
}

// ListOrgs returns all active organizations. SA and ADM only.
//
//encore:api auth method=GET path=/organizations
func ListOrgs(ctx context.Context) (*ListOrgsResponse, error) {
	if err := requireRole(authhandler.RoleSA, authhandler.RoleADM); err != nil {
		return nil, err
	}
	orgs, err := queryActiveOrgs(ctx)
	if err != nil {
		return nil, err
	}

	return &ListOrgsResponse{
		Organizations: orgs,
		Total:         len(orgs),
	}, nil
}

// GetOrg returns a single organization by ID. SA and ADM only.
//
//encore:api auth method=GET path=/organizations/:id
func GetOrg(ctx context.Context, id string) (*GetOrgResponse, error) {
	if err := requireRole(authhandler.RoleSA, authhandler.RoleADM); err != nil {
		return nil, err
	}
	org, err := queryOrgByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &GetOrgResponse{Organization: *org}, nil
}

// UpdateOrg partially updates an organization. SA only.
//
//encore:api auth method=PUT path=/organizations/:id
func UpdateOrg(ctx context.Context, id string, req *UpdateOrgRequest) (*GetOrgResponse, error) {
	if err := requireRole(authhandler.RoleSA); err != nil {
		return nil, err
	}
	if req.Type != nil && !req.Type.IsValid() {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid organization type").Err()
	}

	org, err := updateOrg(ctx, id, req)
	if err != nil {
		return nil, err
	}

	return &GetOrgResponse{Organization: *org}, nil
}

// DeleteOrg soft-deletes an organization by setting is_active=false. SA only.
//
//encore:api auth method=DELETE path=/organizations/:id
func DeleteOrg(ctx context.Context, id string) (*DeleteOrgResponse, error) {
	if err := requireRole(authhandler.RoleSA); err != nil {
		return nil, err
	}
	if err := softDeleteOrg(ctx, id); err != nil {
		return nil, err
	}

	return &DeleteOrgResponse{Message: "organization deleted successfully"}, nil
}

// ════ INTERNAL ════

func insertOrg(ctx context.Context, req *CreateOrgRequest) (*Organization, error) {
	builder := Client.Organization.
		Create().
		SetName(req.Name).
		SetCode(req.Code).
		SetType(string(req.Type))

	if req.ParentID != nil {
		parentUUID, err := uuid.Parse(*req.ParentID)
		if err != nil {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid parent_id format").Err()
		}
		builder = builder.SetParentID(parentUUID)
	}

	row, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, errs.B().Code(errs.AlreadyExists).Msg("organization with this code already exists").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to create organization").Cause(err).Err()
	}

	return entToOrg(row), nil
}

func queryActiveOrgs(ctx context.Context) ([]Organization, error) {
	rows, err := Client.Organization.
		Query().
		Where(organization.IsActiveEQ(true)).
		Order(ent.Asc(organization.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to list organizations").Cause(err).Err()
	}

	orgs := make([]Organization, 0, len(rows))
	for _, row := range rows {
		orgs = append(orgs, *entToOrg(row))
	}

	return orgs, nil
}

func queryOrgByID(ctx context.Context, id string) (*Organization, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid id format").Err()
	}

	row, err := Client.Organization.
		Query().
		Where(organization.IDEQ(uid)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("organization not found").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to get organization").Cause(err).Err()
	}

	return entToOrg(row), nil
}

func updateOrg(ctx context.Context, id string, req *UpdateOrgRequest) (*Organization, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid id format").Err()
	}

	builder := Client.Organization.UpdateOneID(uid)

	if req.Name != nil {
		builder = builder.SetName(*req.Name)
	}
	if req.Code != nil {
		builder = builder.SetCode(*req.Code)
	}
	if req.ParentID != nil {
		parentUUID, err := uuid.Parse(*req.ParentID)
		if err != nil {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid parent_id format").Err()
		}
		builder = builder.SetParentID(parentUUID)
	}
	if req.Type != nil {
		builder = builder.SetType(string(*req.Type))
	}
	if req.IsActive != nil {
		builder = builder.SetIsActive(*req.IsActive)
	}

	row, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Msg("organization not found").Err()
		}
		if ent.IsConstraintError(err) {
			return nil, errs.B().Code(errs.AlreadyExists).Msg("organization with this code already exists").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("failed to update organization").Cause(err).Err()
	}

	return entToOrg(row), nil
}

func softDeleteOrg(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return errs.B().Code(errs.InvalidArgument).Msg("invalid id format").Err()
	}

	exists, err := Client.Organization.
		Query().
		Where(
			organization.IDEQ(uid),
			organization.IsActiveEQ(true),
		).
		Exist(ctx)
	if err != nil {
		return errs.B().Code(errs.Internal).Msg("failed to delete organization").Cause(err).Err()
	}
	if !exists {
		return errs.B().Code(errs.NotFound).Msg("organization not found").Err()
	}

	err = Client.Organization.
		UpdateOneID(uid).
		SetIsActive(false).
		Exec(ctx)
	if err != nil {
		return errs.B().Code(errs.Internal).Msg("failed to delete organization").Cause(err).Err()
	}

	return nil
}

// ════ AUTH HELPERS ════

func getAuthData() (*authhandler.AuthData, error) {
	ad, ok := auth.Data().(*authhandler.AuthData)
	if !ok {
		return nil, errs.B().Code(errs.Unauthenticated).Msg("not authenticated").Err()
	}
	return ad, nil
}

func requireRole(allowed ...authhandler.UserRole) error {
	ad, err := getAuthData()
	if err != nil {
		return err
	}
	for _, r := range allowed {
		if ad.Role == r {
			return nil
		}
	}
	return errs.B().Code(errs.PermissionDenied).Msg("insufficient permissions").Err()
}

// ════ HELPERS ════

// entToOrg maps an ent.Organization to the domain Organization model.
func entToOrg(e *ent.Organization) *Organization {
	var parentID *string
	if e.ParentID != nil {
		s := e.ParentID.String()
		parentID = &s
	}

	return &Organization{
		ID:        e.ID.String(),
		Name:      e.Name,
		Code:      e.Code,
		ParentID:  parentID,
		Type:      OrgType(e.Type),
		IsActive:  e.IsActive,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}
