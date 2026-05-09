package authhandler

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"encore.dev/beta/errs"
	"github.com/golang-jwt/jwt/v5"
)

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func makeRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

func jwksFromPublicKey(kid string, pub *rsa.PublicKey) jwksResponse {
	e := make([]byte, 0)
	eInt := pub.E
	for eInt > 0 {
		e = append([]byte{byte(eInt & 0xff)}, e...)
		eInt >>= 8
	}

	return jwksResponse{
		Keys: []jwkKey{
			{
				Kid: kid,
				Kty: "RSA",
				Alg: "RS256",
				Use: "sig",
				N:   b64url(pub.N.Bytes()),
				E:   b64url(e),
			},
		},
	}
}

func newJWKSHandler(t *testing.T, jwks jwksResponse) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/protocol/openid-connect/certs" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(jwks); err != nil {
			t.Fatalf("encode jwks: %v", err)
		}
	}))
}

func makeValidClaims(issuer, audience string) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"sub":   "kc-user-123",
		"email": "user@example.com",
		"iss":   issuer,
		"aud":   audience,
		"exp":   jwt.NewNumericDate(now.Add(5 * time.Minute)),
		"iat":   jwt.NewNumericDate(now),
		"realm_access": map[string]interface{}{
			"roles": []interface{}{"ADM"},
		},
		"companyId": "company-1",
		"dzoId":     "dzo-1",
	}
}

func signRS256Token(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func signHS256Token(t *testing.T, secret []byte, kid string, claims jwt.MapClaims) string {
	t.Helper()

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	signed, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func resetKeyStore() {
	keyStore.mu.Lock()
	defer keyStore.mu.Unlock()
	keyStore.keys = nil
}

func TestAuthHandler_Success(t *testing.T) {
	resetKeyStore()

	key := makeRSAKey(t)
	kid := "test-kid"

	jwksServer := newJWKSHandler(t, jwksFromPublicKey(kid, &key.PublicKey))
	defer jwksServer.Close()

	secrets.KeycloakIssuerURL = jwksServer.URL
	secrets.KeycloakAudience = "account"

	token := signRS256Token(t, key, kid, makeValidClaims(jwksServer.URL, "account"))

	uid, ad, err := AuthHandler(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(uid) != "kc-user-123" {
		t.Errorf("expected uid kc-user-123, got %q", uid)
	}
	if ad == nil {
		t.Fatal("expected auth data, got nil")
	}
	if ad.Role != RoleADM {
		t.Errorf("expected role ADM, got %q", ad.Role)
	}
	if ad.Email != "user@example.com" {
		t.Errorf("expected email user@example.com, got %q", ad.Email)
	}
}

func TestAuthHandler_InvalidSignature(t *testing.T) {
	resetKeyStore()

	jwksKey := makeRSAKey(t)
	signingKey := makeRSAKey(t)
	kid := "test-kid"

	jwksServer := newJWKSHandler(t, jwksFromPublicKey(kid, &jwksKey.PublicKey))
	defer jwksServer.Close()

	secrets.KeycloakIssuerURL = jwksServer.URL
	secrets.KeycloakAudience = "account"

	token := signRS256Token(t, signingKey, kid, makeValidClaims(jwksServer.URL, "account"))

	_, _, err := AuthHandler(context.Background(), token)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", errs.Code(err))
	}
}

func TestAuthHandler_MissingKid(t *testing.T) {
	resetKeyStore()

	key := makeRSAKey(t)

	jwksServer := newJWKSHandler(t, jwksFromPublicKey("some-kid", &key.PublicKey))
	defer jwksServer.Close()

	secrets.KeycloakIssuerURL = jwksServer.URL
	secrets.KeycloakAudience = "account"

	token := signRS256Token(t, key, "", makeValidClaims(jwksServer.URL, "account"))

	_, _, err := AuthHandler(context.Background(), token)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", errs.Code(err))
	}
}

func TestAuthHandler_UnexpectedSigningMethod(t *testing.T) {
	resetKeyStore()

	key := makeRSAKey(t)
	kid := "test-kid"

	jwksServer := newJWKSHandler(t, jwksFromPublicKey(kid, &key.PublicKey))
	defer jwksServer.Close()

	secrets.KeycloakIssuerURL = jwksServer.URL
	secrets.KeycloakAudience = "account"

	token := signHS256Token(t, []byte("super-secret"), kid, makeValidClaims(jwksServer.URL, "account"))

	_, _, err := AuthHandler(context.Background(), token)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", errs.Code(err))
	}
}

func TestAuthHandler_InvalidIssuer(t *testing.T) {
	resetKeyStore()

	key := makeRSAKey(t)
	kid := "test-kid"

	jwksServer := newJWKSHandler(t, jwksFromPublicKey(kid, &key.PublicKey))
	defer jwksServer.Close()

	secrets.KeycloakIssuerURL = jwksServer.URL
	secrets.KeycloakAudience = "account"

	token := signRS256Token(t, key, kid, makeValidClaims("http://wrong-issuer", "account"))

	_, _, err := AuthHandler(context.Background(), token)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", errs.Code(err))
	}
}

func TestAuthHandler_InvalidAudience(t *testing.T) {
	resetKeyStore()

	key := makeRSAKey(t)
	kid := "test-kid"

	jwksServer := newJWKSHandler(t, jwksFromPublicKey(kid, &key.PublicKey))
	defer jwksServer.Close()

	secrets.KeycloakIssuerURL = jwksServer.URL
	secrets.KeycloakAudience = "account"

	token := signRS256Token(t, key, kid, makeValidClaims(jwksServer.URL, "wrong-audience"))

	_, _, err := AuthHandler(context.Background(), token)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", errs.Code(err))
	}
}

func TestAuthHandler_ExpiredToken(t *testing.T) {
	resetKeyStore()

	key := makeRSAKey(t)
	kid := "test-kid"

	jwksServer := newJWKSHandler(t, jwksFromPublicKey(kid, &key.PublicKey))
	defer jwksServer.Close()

	secrets.KeycloakIssuerURL = jwksServer.URL
	secrets.KeycloakAudience = "account"

	claims := makeValidClaims(jwksServer.URL, "account")
	claims["exp"] = jwt.NewNumericDate(time.Now().Add(-1 * time.Hour))

	token := signRS256Token(t, key, kid, claims)

	_, _, err := AuthHandler(context.Background(), token)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", errs.Code(err))
	}
}

func TestAuthHandler_UnknownKid(t *testing.T) {
	resetKeyStore()

	key := makeRSAKey(t)

	jwksServer := newJWKSHandler(t, jwksFromPublicKey("known-kid", &key.PublicKey))
	defer jwksServer.Close()

	secrets.KeycloakIssuerURL = jwksServer.URL
	secrets.KeycloakAudience = "account"

	token := signRS256Token(t, key, "unknown-kid", makeValidClaims(jwksServer.URL, "account"))

	_, _, err := AuthHandler(context.Background(), token)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errs.Code(err) != errs.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", errs.Code(err))
	}
}

func TestAuthHandler_JWKSUnavailable(t *testing.T) {
	resetKeyStore()

	key := makeRSAKey(t)

	secrets.KeycloakIssuerURL = "http://127.0.0.1:1"
	secrets.KeycloakAudience = "account"

	token := signRS256Token(t, key, "test-kid", makeValidClaims("http://127.0.0.1:1", "account"))

	_, _, err := AuthHandler(context.Background(), token)
	if err == nil {
		t.Fatal("expected error, фgot nil")
	}
	if errs.Code(err) != errs.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", errs.Code(err))
	}
}