package auth

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims used by Conduit.
type Claims struct {
	jwt.RegisteredClaims
	TenantID string   `json:"tenant_id"`
	Roles    []string `json:"roles,omitempty"`
}

// JWTValidator validates JWT tokens.
type JWTValidator struct {
	publicKey     *rsa.PublicKey
	issuer        string
	audience      string
	jwksValidator *JWKSValidator // optional Auth0 JWKS validator
}

// NewJWTValidator creates a new JWT validator.
func NewJWTValidator(publicKey *rsa.PublicKey, issuer, audience string) *JWTValidator {
	return &JWTValidator{
		publicKey: publicKey,
		issuer:    issuer,
		audience:  audience,
	}
}

// SetJWKSValidator adds an Auth0 JWKS validator as primary auth method.
func (v *JWTValidator) SetJWKSValidator(jwks *JWKSValidator) {
	v.jwksValidator = jwks
}

// Validate parses and validates a JWT token string.
// Tries JWKS (Auth0) first if configured, falls back to local key.
func (v *JWTValidator) Validate(tokenString string) (*Claims, error) {
	// Try JWKS validator first (Auth0)
	if v.jwksValidator != nil {
		claims, err := v.jwksValidator.ValidateToken(tokenString)
		if err == nil {
			return claims, nil
		}
		// Fall through to local key validation
	}

	// Local key validation
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.publicKey, nil
	},
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	if claims.TenantID == "" {
		return nil, errors.New("missing tenant_id claim")
	}

	return claims, nil
}

// IssueToken creates a signed JWT for testing/development.
func IssueToken(privateKey *rsa.PrivateKey, issuer, audience, subject, tenantID string, roles []string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		TenantID: tenantID,
		Roles:    roles,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(privateKey)
}
