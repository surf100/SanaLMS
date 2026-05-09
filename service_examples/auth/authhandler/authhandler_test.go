package authhandler

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

func validClaims() jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"sub":   "kc-user-abc-123",
		"email": "user@example.com",
		"iss":   "http://localhost:8080/realms/test-realm",
		"aud":   "account",
		"exp":   jwt.NewNumericDate(now.Add(5 * time.Minute)),
		"iat":   jwt.NewNumericDate(now),
		"realm_access": map[string]interface{}{
			"roles": []interface{}{"SA", "offline_access", "uma_authorization"},
		},
		"companyId": "company-uuid-1",
		"dzoId":     "dzo-uuid-1",
	}
}

func signToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "test-kid"
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

func TestExtractAuthData_ValidClaims(t *testing.T) {
	ad, err := ExtractAuthData(validClaims())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ad.KeycloakUserID != "kc-user-abc-123" {
		t.Errorf("expected sub kc-user-abc-123, got %q", ad.KeycloakUserID)
	}
	if ad.Email != "user@example.com" {
		t.Errorf("expected email user@example.com, got %q", ad.Email)
	}
	if ad.Role != RoleSA {
		t.Errorf("expected role SA, got %q", ad.Role)
	}
	if ad.CompanyID != "company-uuid-1" {
		t.Errorf("expected companyId company-uuid-1, got %q", ad.CompanyID)
	}
	if ad.DzoID != "dzo-uuid-1" {
		t.Errorf("expected dzoId dzo-uuid-1, got %q", ad.DzoID)
	}
}

func TestExtractAuthData_MissingSubject(t *testing.T) {
	c := validClaims()
	delete(c, "sub")
	_, err := ExtractAuthData(c)
	if err == nil {
		t.Fatal("expected error for missing subject, got nil")
	}
	if !errors.Is(err, ErrMissingSubject) {
		t.Errorf("expected ErrMissingSubject, got %v", err)
	}
}

func TestExtractAuthData_EmptySubject(t *testing.T) {
	c := validClaims()
	c["sub"] = ""
	_, err := ExtractAuthData(c)
	if err == nil {
		t.Fatal("expected error for empty subject, got nil")
	}
	if !errors.Is(err, ErrMissingSubject) {
		t.Errorf("expected ErrMissingSubject, got %v", err)
	}
}

func TestExtractAuthData_MissingOptionalFields(t *testing.T) {
	c := jwt.MapClaims{
		"sub": "user-1",
		"exp": jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
	}
	ad, err := ExtractAuthData(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ad.Email != "" {
		t.Errorf("expected empty email, got %q", ad.Email)
	}
	if ad.CompanyID != "" {
		t.Errorf("expected empty companyId, got %q", ad.CompanyID)
	}
	if ad.DzoID != "" {
		t.Errorf("expected empty dzoId, got %q", ad.DzoID)
	}
	if ad.Role != RoleEMP {
		t.Errorf("expected default role EMP, got %q", ad.Role)
	}
}

func TestExtractRole_SA(t *testing.T) {
	c := jwt.MapClaims{"realm_access": map[string]interface{}{"roles": []interface{}{"SA", "offline_access"}}}
	if got := ExtractRole(c); got != RoleSA {
		t.Errorf("expected SA, got %q", got)
	}
}

func TestExtractRole_ADM(t *testing.T) {
	c := jwt.MapClaims{"realm_access": map[string]interface{}{"roles": []interface{}{"ADM", "uma_authorization"}}}
	if got := ExtractRole(c); got != RoleADM {
		t.Errorf("expected ADM, got %q", got)
	}
}

func TestExtractRole_HR(t *testing.T) {
	c := jwt.MapClaims{"realm_access": map[string]interface{}{"roles": []interface{}{"HR"}}}
	if got := ExtractRole(c); got != RoleHR {
		t.Errorf("expected HR, got %q", got)
	}
}

func TestExtractRole_EMP_Explicit(t *testing.T) {
	c := jwt.MapClaims{"realm_access": map[string]interface{}{"roles": []interface{}{"EMP"}}}
	if got := ExtractRole(c); got != RoleEMP {
		t.Errorf("expected EMP, got %q", got)
	}
}

func TestExtractRole_NoRealmAccess(t *testing.T) {
	if got := ExtractRole(jwt.MapClaims{}); got != RoleEMP {
		t.Errorf("expected default EMP, got %q", got)
	}
}

func TestExtractRole_EmptyRoles(t *testing.T) {
	c := jwt.MapClaims{"realm_access": map[string]interface{}{"roles": []interface{}{}}}
	if got := ExtractRole(c); got != RoleEMP {
		t.Errorf("expected default EMP, got %q", got)
	}
}

func TestExtractRole_PriorityOrder_SA_Over_ADM(t *testing.T) {
	c := jwt.MapClaims{"realm_access": map[string]interface{}{"roles": []interface{}{"ADM", "SA", "HR"}}}
	if got := ExtractRole(c); got != RoleSA {
		t.Errorf("expected SA (highest priority), got %q", got)
	}
}

func TestExtractRole_PriorityOrder_ADM_Over_HR(t *testing.T) {
	c := jwt.MapClaims{"realm_access": map[string]interface{}{"roles": []interface{}{"HR", "ADM"}}}
	if got := ExtractRole(c); got != RoleADM {
		t.Errorf("expected ADM over HR, got %q", got)
	}
}

func TestExtractRole_OnlySystemRoles(t *testing.T) {
	c := jwt.MapClaims{"realm_access": map[string]interface{}{"roles": []interface{}{"offline_access", "uma_authorization"}}}
	if got := ExtractRole(c); got != RoleEMP {
		t.Errorf("expected default EMP for system-only roles, got %q", got)
	}
}

func TestUserRole_IsValid(t *testing.T) {
	cases := []struct {
		role UserRole
		want bool
	}{
		{RoleSA, true},
		{RoleADM, true},
		{RoleHR, true},
		{RoleEMP, true},
		{UserRole("UNKNOWN"), false},
		{UserRole(""), false},
	}
	for _, tc := range cases {
		if got := tc.role.IsValid(); got != tc.want {
			t.Errorf("UserRole(%q).IsValid() = %v, want %v", tc.role, got, tc.want)
		}
	}
}

func TestUserRole_Priority(t *testing.T) {
	if RoleSA.Priority() <= RoleADM.Priority() {
		t.Error("SA should have higher priority than ADM")
	}
	if RoleADM.Priority() <= RoleHR.Priority() {
		t.Error("ADM should have higher priority than HR")
	}
	if RoleHR.Priority() <= RoleEMP.Priority() {
		t.Error("HR should have higher priority than EMP")
	}
	if UserRole("INVALID").Priority() != 0 {
		t.Error("invalid role should have priority 0")
	}
}

func TestParseToken_ValidSignature(t *testing.T) {
	key := generateTestKey(t)
	tokenStr := signToken(t, key, validClaims())

	parsed, err := jwt.Parse(tokenStr,
		func(token *jwt.Token) (interface{}, error) { return &key.PublicKey, nil },
		jwt.WithIssuer("http://localhost:8080/realms/test-realm"),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !parsed.Valid {
		t.Error("expected valid token")
	}
}

func TestParseToken_ExpiredToken(t *testing.T) {
	key := generateTestKey(t)
	c := validClaims()
	c["exp"] = jwt.NewNumericDate(time.Now().Add(-1 * time.Hour))

	_, err := jwt.Parse(signToken(t, key, c),
		func(token *jwt.Token) (interface{}, error) { return &key.PublicKey, nil },
		jwt.WithExpirationRequired(),
	)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestParseToken_WrongIssuer(t *testing.T) {
	key := generateTestKey(t)
	c := validClaims()
	c["iss"] = "http://evil.com/realms/bad"

	_, err := jwt.Parse(signToken(t, key, c),
		func(token *jwt.Token) (interface{}, error) { return &key.PublicKey, nil },
		jwt.WithIssuer("http://localhost:8080/realms/test-realm"),
		jwt.WithExpirationRequired(),
	)
	if err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestParseToken_WrongSigningKey(t *testing.T) {
	signingKey := generateTestKey(t)
	wrongKey := generateTestKey(t)

	_, err := jwt.Parse(signToken(t, signingKey, validClaims()),
		func(token *jwt.Token) (interface{}, error) { return &wrongKey.PublicKey, nil },
		jwt.WithExpirationRequired(),
	)
	if err == nil {
		t.Fatal("expected error for wrong signing key")
	}
}

func TestParseToken_CorrectAudience(t *testing.T) {
	key := generateTestKey(t)
	parsed, err := jwt.Parse(signToken(t, key, validClaims()),
		func(token *jwt.Token) (interface{}, error) { return &key.PublicKey, nil },
		jwt.WithIssuer("http://localhost:8080/realms/test-realm"),
		jwt.WithAudience("account"),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		t.Fatalf("unexpected error with correct audience: %v", err)
	}
	if !parsed.Valid {
		t.Error("expected valid token with correct audience")
	}
}

func TestParseToken_WrongAudience(t *testing.T) {
	key := generateTestKey(t)
	_, err := jwt.Parse(signToken(t, key, validClaims()),
		func(token *jwt.Token) (interface{}, error) { return &key.PublicKey, nil },
		jwt.WithIssuer("http://localhost:8080/realms/test-realm"),
		jwt.WithAudience("wrong-client"),
		jwt.WithExpirationRequired(),
	)
	if err == nil {
		t.Fatal("expected error for wrong audience, got nil")
	}
}

func TestParseToken_MissingAudienceClaim(t *testing.T) {
	key := generateTestKey(t)
	c := validClaims()
	delete(c, "aud")

	_, err := jwt.Parse(signToken(t, key, c),
		func(token *jwt.Token) (interface{}, error) { return &key.PublicKey, nil },
		jwt.WithIssuer("http://localhost:8080/realms/test-realm"),
		jwt.WithAudience("account"),
		jwt.WithExpirationRequired(),
	)
	if err == nil {
		t.Fatal("expected error when audience claim is missing, got nil")
	}
}

func TestRsaPublicKeyFromJWK_Valid(t *testing.T) {
	key, err := rsaPublicKeyFromJWK(
		"0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAtVT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn64tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FDW2QvzqY368QQMicAtaSqfF5JA-AQAZ",
		"AQAB",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.E != 65537 {
		t.Errorf("expected exponent 65537, got %d", key.E)
	}
	if key.N == nil {
		t.Error("expected non-nil modulus")
	}
}
