package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/conduit-obs/conduit/internal/auth"
	"github.com/conduit-obs/conduit/internal/ratelimit"
	"github.com/conduit-obs/conduit/internal/tenant"
)

type requestIDKey struct{}

// RequestIDFromContext returns the request ID from context.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// RequestIDMiddleware generates or propagates X-Request-ID on every request.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			b := make([]byte, 16)
			rand.Read(b)
			requestID = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RateLimitMiddleware enforces per-tenant rate limits using a token bucket.
func RateLimitMiddleware(limiter *ratelimit.TokenBucket, getRateLimit func(tenantID string) int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t, ok := tenant.FromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			rl := getRateLimit(t.ID)
			if rl <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			allowed, retryAfter := limiter.Allow(t.ID, rl)
			if !allowed {
				w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter+1))
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type claimsKey struct{}

func withClaims(ctx context.Context, claims *auth.Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}

func claimsFromContext(ctx context.Context) (*auth.Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(*auth.Claims)
	return c, ok
}

// AuthMiddleware validates JWT tokens and injects tenant context.
func AuthMiddleware(validator *auth.JWTValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				http.Error(w, `{"error":"invalid authorization header"}`, http.StatusUnauthorized)
				return
			}

			claims, err := validator.Validate(parts[1])
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			ctx := tenant.WithTenant(r.Context(), &tenant.Tenant{
				ID: claims.TenantID,
			})
			ctx = withClaims(ctx, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CombinedAuthMiddleware tries API key auth first, then falls back to JWT.
func CombinedAuthMiddleware(jwtValidator *auth.JWTValidator, apiKeyMW func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	jwtMW := AuthMiddleware(jwtValidator)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If X-API-Key header present, use API key middleware
			if r.Header.Get("X-API-Key") != "" {
				apiKeyMW(next).ServeHTTP(w, r)
				return
			}
			// Otherwise use JWT
			jwtMW(next).ServeHTTP(w, r)
		})
	}
}

// RequirePermission middleware checks that the authenticated user has the required permission.
func RequirePermission(enforcer *auth.RBACEnforcer, perm auth.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := claimsFromContext(r.Context())
			if !ok {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			if !enforcer.HasPermission(claims.Roles, perm) {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
