package clients

import (
	"context"
	"testing"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"

	"encore.app/auth/authhandler"
)

// ════ HELPERS ════

func withSA(ctx context.Context) context.Context {
	return auth.WithContext(ctx, "sa-user", &authhandler.AuthData{
		KeycloakUserID: "sa-user",
		Role:           authhandler.RoleSA,
	})
}

func withEMP(ctx context.Context) context.Context {
	return auth.WithContext(ctx, "emp-user", &authhandler.AuthData{
		KeycloakUserID: "emp-user",
		Role:           authhandler.RoleEMP,
	})
}

// ════ TESTS ════

func TestCreateClient_Success(t *testing.T) {
	ctx := withSA(context.Background())
	domain := "example.com"
	limit := 100

	lang := "ru"

	resp, err := CreateClient(ctx, &CreateClientRequest{
		Name:      "Test Client",
		Domain:    &domain,
		Language:  &lang,
		UserLimit: &limit,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Client.Name != "Test Client" {
		t.Errorf("expected client name 'Test Client', got %q", resp.Client.Name)
	}
	if resp.Client.Domain == nil || *resp.Client.Domain != "example.com" {
		t.Errorf("expected domain 'example.com'")
	}
	if resp.Client.Language == nil || *resp.Client.Language != "ru" {
		t.Errorf("expected language 'ru'")
	}
	if resp.Client.UserLimit == nil || *resp.Client.UserLimit != 100 {
		t.Errorf("expected user_limit 100")
	}
	if !resp.Client.IsActive {
		t.Error("expected new client to be active")
	}
}

func TestCreateClient_NonSADenied(t *testing.T) {
	ctx := withEMP(context.Background())
	
	_, err := CreateClient(ctx, &CreateClientRequest{
		Name: "Forbidden Client",
	})
	if err == nil {
		t.Fatal("expected error for non-SA user, got nil")
	}
	if errs.Code(err) != errs.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", errs.Code(err))
	}
}

func TestCreateClient_EmptyName(t *testing.T) {
	ctx := withSA(context.Background())
	
	_, err := CreateClient(ctx, &CreateClientRequest{
		Name: "   ",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestCreateClient_InvalidDomain(t *testing.T) {
	ctx := withSA(context.Background())
	domain := "not a domain"
	
	_, err := CreateClient(ctx, &CreateClientRequest{
		Name:   "Bad Domain Client",
		Domain: &domain,
	})
	if err == nil {
		t.Fatal("expected error for invalid domain")
	}
	if errs.Code(err) != errs.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", errs.Code(err))
	}
}

func TestListClients_Success(t *testing.T) {
	ctx := withSA(context.Background())

	// Create a client to ensure there's at least one
	resp, err := CreateClient(ctx, &CreateClientRequest{
		Name: "Listable Client",
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	listResp, err := ListClients(ctx)
	if err != nil {
		t.Fatalf("unexpected error on list: %v", err)
	}

	if listResp.Total == 0 {
		t.Error("expected total > 0")
	}

	found := false
	for _, c := range listResp.Clients {
		if c.ID == resp.Client.ID {
			found = true
			break
		}
	}

	if !found {
		t.Error("recently created client was not returned in list")
	}
}
