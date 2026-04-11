package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"
)

func generateTestKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key, &key.PublicKey
}

func TestJWT_IssueAndValidate(t *testing.T) {
	priv, pub := generateTestKeys(t)
	validator := NewJWTValidator(pub, "conduit", "conduit-api")

	token, err := IssueToken(priv, "conduit", "conduit-api", "user-123", "tenant-abc", []string{"admin"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	claims, err := validator.Validate(token)
	if err != nil {
		t.Fatal(err)
	}

	if claims.TenantID != "tenant-abc" {
		t.Errorf("expected tenant-abc, got %s", claims.TenantID)
	}
	if claims.Subject != "user-123" {
		t.Errorf("expected user-123, got %s", claims.Subject)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
		t.Errorf("expected [admin], got %v", claims.Roles)
	}
}

func TestJWT_ExpiredToken(t *testing.T) {
	priv, pub := generateTestKeys(t)
	validator := NewJWTValidator(pub, "conduit", "conduit-api")

	token, err := IssueToken(priv, "conduit", "conduit-api", "user-123", "tenant-abc", nil, -time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	_, err = validator.Validate(token)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestJWT_WrongIssuer(t *testing.T) {
	priv, pub := generateTestKeys(t)
	validator := NewJWTValidator(pub, "conduit", "conduit-api")

	token, err := IssueToken(priv, "wrong-issuer", "conduit-api", "user-123", "tenant-abc", nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	_, err = validator.Validate(token)
	if err == nil {
		t.Error("expected error for wrong issuer")
	}
}

func TestJWT_WrongAudience(t *testing.T) {
	priv, pub := generateTestKeys(t)
	validator := NewJWTValidator(pub, "conduit", "conduit-api")

	token, err := IssueToken(priv, "conduit", "wrong-audience", "user-123", "tenant-abc", nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	_, err = validator.Validate(token)
	if err == nil {
		t.Error("expected error for wrong audience")
	}
}

func TestJWT_MissingTenantID(t *testing.T) {
	priv, pub := generateTestKeys(t)
	validator := NewJWTValidator(pub, "conduit", "conduit-api")

	token, err := IssueToken(priv, "conduit", "conduit-api", "user-123", "", nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	_, err = validator.Validate(token)
	if err == nil {
		t.Error("expected error for missing tenant_id")
	}
}
