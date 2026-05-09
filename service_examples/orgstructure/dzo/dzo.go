package dzo

import (
	"context"
	"strings"
	"time"

	"encore.app/auth/authhandler"
	"encore.app/db/ent/company"
	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"encore.app/db/ent"
	"encore.app/db/ent/dzoorganization"
	"encore.app/db/ent/employee"
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

// CreateDZO creates a new DZO.
//
//encore:api auth method=POST path=/dzo
func CreateDZO(ctx context.Context, req *CreateDZORequest) (*GetDZOResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}

	if err := requireRole(ad, authhandler.RoleSA, authhandler.RoleADM); err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.Name) == "" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("name is required").Err()
	}
	if strings.TrimSpace(req.ClientID) == "" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("client_id is required").Err()
	}

	// ADM can only create DZO within their own client.
	if ad.Role == authhandler.RoleADM && req.ClientID != ad.CompanyID {
		return nil, errs.B().Code(errs.PermissionDenied).Msg("admin can only create DZO within their own client").Err()
	}

	dzo, err := createDZO(ctx, req)
	if err != nil {
		return nil, err
	}

	return &GetDZOResponse{DZO: *dzo}, nil
}

// ListDZO returns all active DZO. SA sees all; ADM sees only their client's DZOs; HR sees only their own DZO.
//
//encore:api auth method=GET path=/dzo
func ListDZO(ctx context.Context) (*ListDZOResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}

	if err := requireRole(ad, authhandler.RoleSA, authhandler.RoleADM, authhandler.RoleHR); err != nil {
		return nil, err
	}

	var dzos []DZO
	switch ad.Role {
	case authhandler.RoleSA:
		dzos, err = queryActiveDZO(ctx)
	case authhandler.RoleHR:
		// HR sees only their own DZO (from token)
		if ad.DzoID == "" {
			dzos = []DZO{}
		} else {
			dzos, err = queryActiveDZOByID(ctx, ad.DzoID)
		}
	default:
		// ADM: scoped to their client
		if ad.CompanyID == "" {
			dzos = []DZO{}
		} else {
			dzos, err = queryActiveDZOByClient(ctx, ad.CompanyID)
		}
	}
	if err != nil {
		return nil, err
	}

	if ad.Role == authhandler.RoleHR {
		filtered := make([]DZO, 0)
		for _, d := range dzos {
			if d.ID == ad.DzoID {
				filtered = append(filtered, d)
				break
			}
		}
		return &ListDZOResponse{
			DZOs:  filtered,
			Total: len(filtered),
		}, nil
	}

	return &ListDZOResponse{
		DZOs:  dzos,
		Total: len(dzos),
	}, nil
}

// GetDZO returns DZO by ID. ADM is scoped to their client.
//
//encore:api auth method=GET path=/dzo/:id
func GetDZO(ctx context.Context, id string) (*GetDZOResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}

	if err := requireRole(ad, authhandler.RoleSA, authhandler.RoleADM); err != nil {
		return nil, err
	}

	dzo, err := queryDZOByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if ad.Role == authhandler.RoleADM && dzo.ClientID != ad.CompanyID {
		return nil, errs.B().Code(errs.NotFound).Err()
	}

	return &GetDZOResponse{DZO: *dzo}, nil
}

// UpdateDZO updates DZO. ADM is scoped to their client.
//
//encore:api auth method=PATCH path=/dzo/:id
func UpdateDZO(ctx context.Context, id string, req *UpdateDZORequest) (*GetDZOResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}

	if err := requireRole(ad, authhandler.RoleSA, authhandler.RoleADM); err != nil {
		return nil, err
	}

	// For ADM, verify the DZO belongs to their client before updating.
	if ad.Role == authhandler.RoleADM {
		existing, err := queryDZOByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if existing.ClientID != ad.CompanyID {
			return nil, errs.B().Code(errs.NotFound).Err()
		}
	}

	dzo, err := updateDZO(ctx, id, req)
	if err != nil {
		return nil, err
	}

	return &GetDZOResponse{DZO: *dzo}, nil
}

// DeleteDZO soft deletes DZO. ADM is scoped to their client.
//
//encore:api auth method=DELETE path=/dzo/:id
func DeleteDZO(ctx context.Context, id string) (*DeleteDZOResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}

	if err := requireRole(ad, authhandler.RoleSA, authhandler.RoleADM); err != nil {
		return nil, err
	}

	// For ADM, verify the DZO belongs to their client before deleting.
	if ad.Role == authhandler.RoleADM {
		existing, err := queryDZOByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if existing.ClientID != ad.CompanyID {
			return nil, errs.B().Code(errs.NotFound).Err()
		}
	}

	count, err := deleteDZO(ctx, id)
	if err != nil {
		return nil, err
	}

	return &DeleteDZOResponse{
		Message:        "dzo deleted",
		EmployeesCount: count,
	}, nil
}

// ════ INTERNAL ════

func createDZO(ctx context.Context, req *CreateDZORequest) (*DZO, error) {
	clientID, err := uuid.Parse(req.ClientID)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid client_id").Err()
	}
	//client exist check
	err = clientExists(ctx, clientID)
	if err != nil {
		return nil, err
	}

	normalizedName := strings.TrimSpace(req.Name)
	if normalizedName == "" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("name is required").Err()
	}

	// uniqueness check
	exists, err := Client.DzoOrganization.
		Query().
		Where(dzoorganization.NameEQ(normalizedName)).
		Exist(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Err()
	}
	if exists {
		return nil, errs.B().Code(errs.AlreadyExists).Msg("dzo already exists").Err()
	}

	row, err := Client.DzoOrganization.
		Create().
		SetID(uuid.New()).
		SetClientID(clientID).
		SetName(normalizedName).
		SetNillableShortName(req.ShortName).
		SetNillableBin(req.BIN).
		SetCreatedAt(time.Now()).
		SetUpdatedAt(time.Now()).
		Save(ctx)

	if err != nil {
		return nil, errs.B().Code(errs.Internal).Err()
	}

	return entToDZO(row, 0), nil
}

func clientExists(ctx context.Context, clientID uuid.UUID) error {
	exists, err := Client.Company.Query().Where(company.ID(clientID)).Exist(ctx)
	if err != nil {
		return errs.B().Code(errs.Internal).Err()
	}
	if !exists {
		return errs.B().Code(errs.NotFound).Msg("client doesn't exist").Err()
	}
	return nil
}

func queryActiveDZO(ctx context.Context) ([]DZO, error) {
	rows, err := Client.DzoOrganization.
		Query().
		Where(dzoorganization.IsActiveEQ(true)).
		All(ctx)

	if err != nil {
		return nil, errs.B().Code(errs.Internal).Err()
	}

	res := make([]DZO, 0, len(rows))
	for _, r := range rows {
		res = append(res, *entToDZO(r, 0))
	}

	return res, nil
}

func queryActiveDZOByClient(ctx context.Context, clientID string) ([]DZO, error) {
	clientUUID, err := uuid.Parse(clientID)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid client_id format").Err()
	}

	rows, err := Client.DzoOrganization.
		Query().
		Where(
			dzoorganization.IsActiveEQ(true),
			dzoorganization.ClientIDEQ(clientUUID),
		).
		All(ctx)

	if err != nil {
		return nil, errs.B().Code(errs.Internal).Err()
	}

	res := make([]DZO, 0, len(rows))
	for _, r := range rows {
		res = append(res, *entToDZO(r, 0))
	}

	return res, nil
}

func queryActiveDZOByID(ctx context.Context, dzoID string) ([]DZO, error) {
	dzoUUID, err := uuid.Parse(dzoID)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid dzo_id format").Err()
	}

	rows, err := Client.DzoOrganization.
		Query().
		Where(
			dzoorganization.IsActiveEQ(true),
			dzoorganization.IDEQ(dzoUUID),
		).
		All(ctx)

	if err != nil {
		return nil, errs.B().Code(errs.Internal).Err()
	}

	res := make([]DZO, 0, len(rows))
	for _, r := range rows {
		res = append(res, *entToDZO(r, 0))
	}

	return res, nil
}

func queryDZOByID(ctx context.Context, id string) (*DZO, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Err()
	}

	row, err := Client.DzoOrganization.
		Query().
		Where(dzoorganization.IDEQ(uid)).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Err()
		}
		return nil, errs.B().Code(errs.Internal).Err()
	}

	empCount, _ := Client.Employee.
		Query().
		Where(employee.DzoIDEQ(uid), employee.IsDeletedEQ(false)).
		Count(ctx)

	return entToDZO(row, empCount), nil
}

func updateDZO(ctx context.Context, id string, req *UpdateDZORequest) (*DZO, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Err()
	}

	builder := Client.DzoOrganization.UpdateOneID(uid).SetUpdatedAt(time.Now())

	if req.Name != nil {
		normalizedName := strings.TrimSpace(*req.Name)
		if normalizedName == "" {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("name is required").Err()
		}

		exists, err := Client.DzoOrganization.
			Query().
			Where(
				dzoorganization.NameEQ(normalizedName),
				dzoorganization.IDNEQ(uid),
			).
			Exist(ctx)
		if err != nil {
			return nil, errs.B().Code(errs.Internal).Err()
		}
		if exists {
			return nil, errs.B().Code(errs.AlreadyExists).Msg("dzo already exists").Err()
		}
		builder.SetName(normalizedName)
	}
	if req.ShortName != nil {
		builder.SetShortName(*req.ShortName)
	}
	if req.BIN != nil {
		builder.SetBin(*req.BIN)
	}
	if req.IsActive != nil {
		builder.SetIsActive(*req.IsActive)
	}

	row, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.B().Code(errs.NotFound).Err()
		}
		return nil, errs.B().Code(errs.Internal).Err()
	}

	empCount, _ := Client.Employee.
		Query().
		Where(employee.DzoIDEQ(uid), employee.IsDeletedEQ(false)).
		Count(ctx)

	return entToDZO(row, empCount), nil
}

func deleteDZO(ctx context.Context, id string) (int, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return 0, errs.B().Code(errs.InvalidArgument).Err()
	}

	count, err := Client.Employee.
		Query().
		Where(employee.DzoIDEQ(uid)).
		Count(ctx)
	if err != nil {
		return 0, errs.B().Code(errs.Internal).Err()
	}

	if count > 0 {
		return count, errs.B().
			Code(errs.FailedPrecondition).
			Msg("cannot delete dzo with employees").
			Err()
	}

	err = Client.DzoOrganization.
		UpdateOneID(uid).
		SetIsActive(false).
		Exec(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return 0, errs.B().Code(errs.NotFound).Err()
		}
		return 0, errs.B().Code(errs.Internal).Err()
	}

	return count, nil
}

func requireRole(ad *authhandler.AuthData, allowed ...authhandler.UserRole) error {
	for _, r := range allowed {
		if ad.Role == r {
			return nil
		}
	}
	return errs.B().Code(errs.PermissionDenied).Msg("insufficient permissions").Err()
}

func getAuthData() (*authhandler.AuthData, error) {
	ad, ok := auth.Data().(*authhandler.AuthData)
	if !ok {
		return nil, errs.B().Code(errs.Unauthenticated).Msg("not authenticated").Err()
	}
	return ad, nil
}

// helper

func entToDZO(e *ent.DzoOrganization, employeesCount int) *DZO {
	return &DZO{
		ID:             e.ID.String(),
		ClientID:       e.ClientID.String(),
		Name:           e.Name,
		ShortName:      e.ShortName,
		BIN:            e.Bin,
		IsActive:       e.IsActive,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
		EmployeesCount: employeesCount,
	}
}
