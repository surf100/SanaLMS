package clients

import (
	"context"
	"net/url"
	"strings"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"encore.app/auth/authhandler"
	"encore.app/db/ent"
	"encore.app/db/ent/company"
)

// ════ DATABASE ════

var (
	db       = sqldb.Named("lms")
	dbClient = newEntClient()
)

func newEntClient() *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, db.Stdlib())
	return ent.NewClient(ent.Driver(drv))
}

// ════ ENDPOINTS ════

// CreateClient creates a new company client.
//
//encore:api auth method=POST path=/clients
func CreateClient(ctx context.Context, req *CreateClientRequest) (*GetClientResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}
	if ad.Role != authhandler.RoleSA {
		return nil, errs.B().Code(errs.PermissionDenied).Msg("only super admin can create clients").Err()
	}

	if strings.TrimSpace(req.Name) == "" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("name is required").Err()
	}
	if req.Domain != nil && strings.TrimSpace(*req.Domain) != "" {
		domainVal := strings.TrimSpace(*req.Domain)
		if _, err := url.ParseRequestURI("https://" + domainVal); err != nil {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("domain format is invalid").Err()
		}
	}
	if req.UserLimit != nil && *req.UserLimit <= 0 {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("user_limit must be positive if provided").Err()
	}

	c, err := insertClient(ctx, req)
	if err != nil {
		return nil, err
	}

	return &GetClientResponse{Client: *c}, nil
}

// ListClients returns all active clients.
//
//encore:api auth method=GET path=/clients
func ListClients(ctx context.Context) (*ListClientsResponse, error) {
	ad, err := getAuthData()
	if err != nil {
		return nil, err
	}
	if ad.Role != authhandler.RoleSA {
		return nil, errs.B().Code(errs.PermissionDenied).Msg("only super admin can list all clients").Err()
	}

	c, err := queryClients(ctx)
	if err != nil {
		return nil, err
	}
	return &ListClientsResponse{Clients: c, Total: len(c)}, nil
}

// ════ INTERNAL ════

func getAuthData() (*authhandler.AuthData, error) {
	ad, ok := auth.Data().(*authhandler.AuthData)
	if !ok {
		return nil, errs.B().Code(errs.Unauthenticated).Msg("not authenticated").Err()
	}
	return ad, nil
}

func insertClient(ctx context.Context, req *CreateClientRequest) (*ClientResponseDomain, error) {
	builder := dbClient.Company.
		Create().
		SetName(strings.TrimSpace(req.Name))

	if req.Domain != nil && strings.TrimSpace(*req.Domain) != "" {
		domainStr := strings.TrimSpace(*req.Domain)
		builder = builder.SetDomain(domainStr)
	}
	if req.Language != nil && strings.TrimSpace(*req.Language) != "" {
		lang := strings.TrimSpace(*req.Language)
		builder = builder.SetLanguage(lang)
	}
	if req.UserLimit != nil {
		builder = builder.SetUserLimit(*req.UserLimit)
	}

	row, err := builder.Save(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to create client").Cause(err).Err()
	}

	return entToClient(row), nil
}

func queryClients(ctx context.Context) ([]ClientResponseDomain, error) {
	rows, err := dbClient.Company.Query().
		Where(company.IsActiveEQ(true)).
		Order(ent.Desc(company.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to list clients").Cause(err).Err()
	}

	clients := make([]ClientResponseDomain, 0, len(rows))
	for _, row := range rows {
		clients = append(clients, *entToClient(row))
	}
	return clients, nil
}

// Renamed locally to avoid conflict with Ent Client struct
type ClientResponseDomain = Client

func entToClient(e *ent.Company) *ClientResponseDomain {
	var domain *string
	if e.Domain != nil {
		d := *e.Domain
		domain = &d
	}
	var language *string
	if e.Language != nil {
		l := *e.Language
		language = &l
	}
	var ul *int
	if e.UserLimit != nil {
		u := *e.UserLimit
		ul = &u
	}
	return &ClientResponseDomain{
		ID:        e.ID.String(),
		Name:      e.Name,
		Domain:    domain,
		Language:  language,
		UserLimit: ul,
		IsActive:  e.IsActive,
		CreatedAt: e.CreatedAt,
	}
}
