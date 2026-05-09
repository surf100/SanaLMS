package clients

import (
	"context"
	"testing"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/google/uuid"

	"encore.app/auth/authhandler"
)

// ════ HELPERS ════

func bgCtx() context.Context {
	return context.Background()
}

func withRole(role authhandler.UserRole) context.Context {
	return auth.WithContext(
		bgCtx(),
		auth.UID("user-"+string(role)),
		&authhandler.AuthData{
			KeycloakUserID: "user-" + string(role),
			Role:           role,
		},
	)
}

func saCtx() context.Context {
	return withRole(authhandler.RoleSA)
}

func admCtx() context.Context {
	return withRole(authhandler.RoleADM)
}

func hrCtx() context.Context {
	return withRole(authhandler.RoleHR)
}

func strP(s string) *string {
	return &s
}

func intP(i int) *int {
	return &i
}

func uniqueClientName() string {
	return "client-" + uuid.NewString()
}

func makeClient(t *testing.T) *Client {
	t.Helper()

	resp, err := CreateClient(
		saCtx(),
		&CreateClientRequest{
			Name: uniqueClientName(),
		},
	)
	if err != nil {
		t.Fatalf("makeClient: %v", err)
	}

	return &resp.Client
}

// ════ CREATE — additional cases ════

func TestCreateClient_MinimalFields(t *testing.T) {
	resp, err := CreateClient(
		saCtx(),
		&CreateClientRequest{
			Name: uniqueClientName(),
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Client.Domain != nil {
		t.Error("expected nil domain for minimal request")
	}

	if resp.Client.Language != nil {
		t.Error("expected nil language for minimal request")
	}

	if resp.Client.UserLimit != nil {
		t.Error("expected nil user_limit for minimal request")
	}

	if !resp.Client.IsActive {
		t.Error("new client must be active")
	}
}

func TestCreateClient_ZeroUserLimit(t *testing.T) {
	_, err := CreateClient(
		saCtx(),
		&CreateClientRequest{
			Name:      uniqueClientName(),
			UserLimit: intP(0),
		},
	)

	if err == nil {
		t.Fatal("expected error for zero user_limit")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateClient_NegativeUserLimit(t *testing.T) {
	_, err := CreateClient(
		saCtx(),
		&CreateClientRequest{
			Name:      uniqueClientName(),
			UserLimit: intP(-5),
		},
	)

	if err == nil {
		t.Fatal("expected error for negative user_limit")
	}

	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateClient_EmptyDomainIgnored(t *testing.T) {
	empty := "  "

	resp, err := CreateClient(
		saCtx(),
		&CreateClientRequest{
			Name:   uniqueClientName(),
			Domain: &empty,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Client.Domain != nil && *resp.Client.Domain != "" {
		t.Errorf(
			"expected domain ignored, got %q",
			*resp.Client.Domain,
		)
	}
}

// ════ LIST — role checks ════

func TestListClients_ADMDenied(t *testing.T) {
	_, err := ListClients(admCtx())

	if err == nil {
		t.Fatal("expected error for ADM listing clients")
	}

	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf(
			"expected PermissionDenied, got %v",
			errs.Code(err),
		)
	}
}

func TestListClients_HRDenied(t *testing.T) {
	_, err := ListClients(hrCtx())

	if err == nil {
		t.Fatal("expected error for HR listing clients")
	}

	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf(
			"expected PermissionDenied, got %v",
			errs.Code(err),
		)
	}
}

func TestListClients_TotalMatchesClients(t *testing.T) {
	resp, err := ListClients(saCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Total != len(resp.Clients) {
		t.Errorf(
			"Total %d does not match len(Clients) %d",
			resp.Total,
			len(resp.Clients),
		)
	}
}

// ════ FULL LIFECYCLE ════

func TestClient_CreateAndAppearInList(t *testing.T) {
	name := uniqueClientName()
	limit := 50

	createResp, err := CreateClient(
		saCtx(),
		&CreateClientRequest{
			Name:      name,
			Language:  strP("kz"),
			UserLimit: &limit,
		},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	createdID := createResp.Client.ID

	listResp, err := ListClients(saCtx())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	var found *Client

	for i := range listResp.Clients {
		if listResp.Clients[i].ID == createdID {
			found = &listResp.Clients[i]
			break
		}
	}

	if found == nil {
		t.Fatal("created client not found in list")
	}

	if found.Name != name {
		t.Errorf(
			"expected name %q, got %q",
			name,
			found.Name,
		)
	}

	if found.Language == nil || *found.Language != "kz" {
		t.Error("expected language 'kz'")
	}

	if found.UserLimit == nil || *found.UserLimit != limit {
		t.Errorf("expected user_limit %d", limit)
	}

	if !found.IsActive {
		t.Error("expected newly created client to be active")
	}
}
