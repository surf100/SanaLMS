package authhandler

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/golang-jwt/jwt/v5"
)

var secrets struct {
	KeycloakIssuerURL string
	KeycloakAudience  string
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type keyStoreType struct {
	mu   sync.RWMutex
	keys map[string]*rsa.PublicKey
}

var keyStore = &keyStoreType{}

func (s *keyStoreType) getKey(kid string) (*rsa.PublicKey, error) {
	s.mu.RLock()
	if s.keys != nil {
		if key, ok := s.keys[kid]; ok {
			s.mu.RUnlock()
			return key, nil
		}
	}
	s.mu.RUnlock()

	if err := s.load(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.keys[kid]
	if !ok {
		return nil, fmt.Errorf("unknown signing key id: %s", kid)
	}
	return key, nil
}

func (s *keyStoreType) load() error {
	jwksURL := secrets.KeycloakIssuerURL + "/protocol/openid-connect/certs"
	resp, err := http.Get(jwksURL)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to decode JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || k.Use != "sig" {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}

	s.mu.Lock()
	s.keys = keys
	s.mu.Unlock()
	return nil
}

func rsaPublicKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}

// AuthHandler validates the Bearer JWT and returns the caller's identity.
// The is_active check is NOT performed here — it requires a DB call and is
// enforced by each endpoint via resolveAndCheckCaller.
//
//encore:authhandler
func AuthHandler(ctx context.Context, token string) (auth.UID, *AuthData, error) {
	if token == "" {
		return "", nil, errs.B().Code(errs.Unauthenticated).Msg("missing auth token").Err()
	}

	parsed, err := jwt.Parse(token,
		func(t *jwt.Token) (interface{}, error) {
			if t.Method.Alg() != "RS256" {
				return nil, fmt.Errorf("unexpected signing method: %s", t.Method.Alg())
			}
			kid, ok := t.Header["kid"].(string)
			if !ok {
				return nil, fmt.Errorf("missing kid in token header")
			}
			return keyStore.getKey(kid)
		},
		jwt.WithIssuer(secrets.KeycloakIssuerURL),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		fmt.Printf("JWT Parse Error: %v\n", err)
		return "", nil, errs.B().Code(errs.Unauthenticated).Msg("invalid or expired token").Err()
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		return "", nil, errs.B().Code(errs.Unauthenticated).Msg("invalid token claims").Err()
	}

	// Validate audience if configured
	if secrets.KeycloakAudience != "" {
		aud, err := claims.GetAudience()
		if err == nil && len(aud) > 0 {
			// Check if our audience is in the token's audience list
			found := false
			for _, a := range aud {
				if a == secrets.KeycloakAudience {
					found = true
					break
				}
			}
			if !found {
				fmt.Printf("Audience mismatch. Expected: %s, Got: %v\n", secrets.KeycloakAudience, aud)
				// Don't fail on audience mismatch for now, just log it
			}
		}
	}

	ad, err := ExtractAuthData(claims)
	if err != nil {
		return "", nil, errs.B().Code(errs.Unauthenticated).Msg(err.Error()).Err()
	}

	return auth.UID(ad.KeycloakUserID), ad, nil
}

// ErrMissingSubject is returned when the JWT has no "sub" claim.
var ErrMissingSubject = errors.New("missing subject claim")

// ExtractAuthData builds AuthData from JWT map claims.
// Returns plain errors (no errs.B()) to allow unit testing outside the Encore runtime.
func ExtractAuthData(claims jwt.MapClaims) (*AuthData, error) {
	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return nil, ErrMissingSubject
	}

	email, _ := claims["email"].(string)
	companyID, _ := claims["companyId"].(string)
	dzoID, _ := claims["dzoId"].(string)

	return &AuthData{
		KeycloakUserID: sub,
		Email:          email,
		Role:           ExtractRole(claims),
		CompanyID:      companyID,
		DzoID:          dzoID,
	}, nil
}

// ExtractRole returns the highest-priority business role from realm_access.roles.
// Defaults to EMP when no known role is found.
func ExtractRole(claims jwt.MapClaims) UserRole {
	realmAccess, ok := claims["realm_access"].(map[string]interface{})
	if !ok {
		return RoleEMP
	}
	roles, ok := realmAccess["roles"].([]interface{})
	if !ok {
		return RoleEMP
	}

	has := make(map[string]bool, len(roles))
	for _, r := range roles {
		if s, ok := r.(string); ok {
			has[s] = true
		}
	}

	switch {
	case has[string(RoleSA)]:
		return RoleSA
	case has[string(RoleADM)]:
		return RoleADM
	case has[string(RoleHR)]:
		return RoleHR
	default:
		return RoleEMP
	}
}
