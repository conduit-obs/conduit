package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/conduit-obs/conduit/internal/auth"
)

// Auth0Config returns the Auth0 configuration for the frontend SDK.
func (h *Handlers) Auth0Config(w http.ResponseWriter, r *http.Request) {
	domain := os.Getenv("CONDUIT_AUTH0_DOMAIN")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"enabled":      domain != "",
		"domain":       domain,
		"client_id":    os.Getenv("CONDUIT_AUTH0_CLIENT_ID"),
		"audience":     os.Getenv("CONDUIT_AUTH0_AUDIENCE"),
		"redirect_uri": os.Getenv("CONDUIT_AUTH0_REDIRECT_URI"),
	})
}

// Auth0Callback handles the Auth0 authorization code exchange and user sync.
func (h *Handlers) Auth0Callback(w http.ResponseWriter, r *http.Request) {
	domain := os.Getenv("CONDUIT_AUTH0_DOMAIN")
	clientID := os.Getenv("CONDUIT_AUTH0_CLIENT_ID")

	if domain == "" || clientID == "" {
		http.Error(w, `{"error":"Auth0 not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Code         string `json:"code"`
		CodeVerifier string `json:"code_verifier"`
		RedirectURI  string `json:"redirect_uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		http.Error(w, `{"error":"code is required"}`, http.StatusBadRequest)
		return
	}

	// Exchange code for tokens with Auth0
	tokenURL := fmt.Sprintf("https://%s/oauth/token", domain)
	exchangeBody, _ := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     clientID,
		"code":          req.Code,
		"redirect_uri":  req.RedirectURI,
		"code_verifier": req.CodeVerifier,
	})

	tokenResp, err := http.Post(tokenURL, "application/json", bytes.NewReader(exchangeBody))
	if err != nil {
		http.Error(w, `{"error":"failed to exchange code with Auth0"}`, http.StatusBadGateway)
		return
	}
	defer tokenResp.Body.Close()

	tokenBody, _ := io.ReadAll(tokenResp.Body)

	if tokenResp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf(`{"error":"Auth0 token exchange failed: %s"}`, string(tokenBody)), http.StatusBadGateway)
		return
	}

	var tokenResult struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	json.Unmarshal(tokenBody, &tokenResult)

	// Decode ID token to get user info (we trust Auth0 signed it)
	email, sub, name := decodeIDTokenClaims(tokenResult.IDToken)
	if email == "" {
		http.Error(w, `{"error":"could not extract email from Auth0 token"}`, http.StatusBadRequest)
		return
	}

	// Look up user by email
	user, err := h.repo.GetUserByEmail(r.Context(), email)
	isNewUser := err != nil

	if isNewUser {
		// Auto-provision: create tenant, org, project, environments, user
		tenantName := emailDomain(email)
		if tenantName == "" {
			tenantName = "Personal"
		}

		tenant, err := h.repo.CreateTenant(r.Context(), tenantName)
		if err != nil {
			http.Error(w, `{"error":"failed to provision tenant"}`, http.StatusInternalServerError)
			return
		}

		// Create organization
		h.repo.CreateOrganization(r.Context(), tenant.ID, tenantName)

		// Create environments
		for _, env := range []string{"dev", "staging", "production"} {
			h.repo.CreateEnvironment(r.Context(), tenant.ID, env)
		}

		// Create user with auth0_sub
		user, err = h.repo.CreateUserWithAuth0Sub(r.Context(), tenant.ID, email, sub, name, []string{"admin"})
		if err != nil {
			http.Error(w, `{"error":"failed to create user"}`, http.StatusInternalServerError)
			return
		}

		h.publishAudit(r.Context(), tenant.ID, "tenant.provisioned", map[string]any{
			"tenant_id": tenant.ID,
			"email":     email,
			"auth0_sub": sub,
		})
	}

	// Issue Conduit JWT with tenant context
	var conduitToken string
	if h.jwtPrivateKey != nil {
		conduitToken, _ = auth.IssueToken(h.jwtPrivateKey, h.jwtIssuer, h.jwtAudience, email, user.TenantID, user.Roles, 24*time.Hour)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token":  tokenResult.AccessToken,
		"conduit_token": conduitToken,
		"tenant_id":     user.TenantID,
		"is_new_user":   isNewUser,
		"user": map[string]any{
			"id":    user.ID,
			"email": user.Email,
			"roles": user.Roles,
		},
	})
}

// decodeIDTokenClaims extracts basic claims from a JWT ID token without full validation
// (Auth0 already validated it; we just need the claims).
func decodeIDTokenClaims(idToken string) (email, sub, name string) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return "", "", ""
	}
	// Decode payload (part 1)
	payload := parts[1]
	// Add padding
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return "", "", ""
	}
	var claims struct {
		Email string `json:"email"`
		Sub   string `json:"sub"`
		Name  string `json:"name"`
	}
	json.Unmarshal(decoded, &claims)
	return claims.Email, claims.Sub, claims.Name
}

func emailDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	domain := parts[1]
	// Strip common email providers
	common := map[string]bool{"gmail.com": true, "yahoo.com": true, "hotmail.com": true, "outlook.com": true}
	if common[domain] {
		return parts[0] + "'s workspace"
	}
	return strings.Split(domain, ".")[0]
}
