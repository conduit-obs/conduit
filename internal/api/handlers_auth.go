package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/conduit-obs/conduit/internal/auth"
	"github.com/conduit-obs/conduit/internal/db"
	"github.com/conduit-obs/conduit/internal/tenant"
	"golang.org/x/crypto/bcrypt"
)

// --- Auth Status ---

// AuthStatus returns whether the system has been initialized.
func (h *Handlers) AuthStatus(w http.ResponseWriter, r *http.Request) {
	initialized := false
	if h.repo != nil {
		count, err := h.repo.CountTenants(r.Context())
		if err == nil && count > 0 {
			initialized = true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"initialized": initialized})
}

// --- Auth Setup (first admin bootstrap) ---

// AuthSetup creates the first tenant and admin user. Only works on fresh installations.
func (h *Handlers) AuthSetup(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	// Check if already initialized
	count, err := h.repo.CountTenants(r.Context())
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Error(w, `{"error":"system already initialized"}`, http.StatusConflict)
		return
	}

	var req struct {
		TenantName string `json:"tenant_name"`
		AdminEmail string `json:"admin_email"`
		AdminPassword string `json:"admin_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.TenantName == "" || req.AdminEmail == "" || req.AdminPassword == "" {
		http.Error(w, `{"error":"tenant_name, admin_email, and admin_password are required"}`, http.StatusBadRequest)
		return
	}
	if len(req.AdminPassword) < 8 {
		http.Error(w, `{"error":"password must be at least 8 characters"}`, http.StatusBadRequest)
		return
	}

	// Create tenant
	t, err := h.repo.CreateTenant(r.Context(), req.TenantName)
	if err != nil {
		http.Error(w, `{"error":"failed to create tenant"}`, http.StatusInternalServerError)
		return
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"failed to hash password"}`, http.StatusInternalServerError)
		return
	}

	// Create admin user
	user, err := h.repo.CreateUser(r.Context(), t.ID, req.AdminEmail, string(hash), []string{"admin"})
	if err != nil {
		http.Error(w, `{"error":"failed to create admin user"}`, http.StatusInternalServerError)
		return
	}

	// Issue JWT
	var token string
	if h.jwtPrivateKey != nil {
		token, err = auth.IssueToken(h.jwtPrivateKey, h.jwtIssuer, h.jwtAudience, user.Email, t.ID, user.Roles, 24*time.Hour)
		if err != nil {
			http.Error(w, `{"error":"failed to issue token"}`, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"token":     token,
		"tenant_id": t.ID,
		"user": map[string]any{
			"id":    user.ID,
			"email": user.Email,
			"roles": user.Roles,
		},
	})
}

// --- Auth Login ---

var (
	loginAttempts   = make(map[string]int)
	loginAttemptsMu sync.Mutex
)

// AuthLogin authenticates with email/password and returns a JWT.
func (h *Handlers) AuthLogin(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email and password are required"}`, http.StatusBadRequest)
		return
	}

	// Rate limit check
	loginAttemptsMu.Lock()
	attempts := loginAttempts[req.Email]
	if attempts >= 5 {
		loginAttemptsMu.Unlock()
		http.Error(w, `{"error":"too many login attempts, try again later"}`, http.StatusTooManyRequests)
		return
	}
	loginAttemptsMu.Unlock()

	user, err := h.repo.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		loginAttemptsMu.Lock()
		loginAttempts[req.Email]++
		loginAttemptsMu.Unlock()
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	if user.Status != "active" {
		http.Error(w, `{"error":"account is not active"}`, http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		loginAttemptsMu.Lock()
		loginAttempts[req.Email]++
		loginAttemptsMu.Unlock()
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	// Clear failed attempts on success
	loginAttemptsMu.Lock()
	delete(loginAttempts, req.Email)
	loginAttemptsMu.Unlock()

	// Issue JWT
	var token string
	if h.jwtPrivateKey != nil {
		token, _ = auth.IssueToken(h.jwtPrivateKey, h.jwtIssuer, h.jwtAudience, user.Email, user.TenantID, user.Roles, 15*time.Minute)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token": token,
		"expires_in":   900,
		"user": map[string]any{
			"id":        user.ID,
			"email":     user.Email,
			"roles":     user.Roles,
			"tenant_id": user.TenantID,
		},
	})
}

// AuthRefresh stub — returns same token structure.
func (h *Handlers) AuthRefresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "refresh not yet implemented"})
}

// --- User Invitation ---

// InviteUser creates a pending user with an invite token.
func (h *Handlers) InviteUser(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Email string   `json:"email"`
		Roles []string `json:"roles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, `{"error":"email is required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Roles) == 0 {
		req.Roles = []string{"viewer"}
	}

	claims, _ := claimsFromContext(r.Context())
	invitedBy := ""
	if claims != nil {
		invitedBy = claims.Subject
	}

	// Generate invite token
	inviteToken := generateInviteToken()

	user, err := h.repo.CreateInvitedUser(r.Context(), t.ID, req.Email, inviteToken, invitedBy, req.Roles)
	if err != nil {
		http.Error(w, `{"error":"failed to create invitation"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"user":         user,
		"invite_token": inviteToken,
		"invite_url":   "/auth/accept-invite?token=" + inviteToken,
	})
}

// ListUsers returns all users for the tenant.
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}

	users, err := h.repo.ListUsers(r.Context(), t.ID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if users == nil {
		users = []db.UserRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// UpdateUserHandler updates a user's roles or status.
func (h *Handlers) UpdateUserHandler(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	userID := r.PathValue("id")
	var req struct {
		Roles  []string `json:"roles"`
		Status string   `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	user, err := h.repo.UpdateUser(r.Context(), userID, req.Roles, req.Status)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// AcceptInvite accepts an invite token and sets the user's password.
func (h *Handlers) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Token    string `json:"invite_token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" || req.Password == "" {
		http.Error(w, `{"error":"invite_token and password are required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Password) < 8 {
		http.Error(w, `{"error":"password must be at least 8 characters"}`, http.StatusBadRequest)
		return
	}

	user, err := h.repo.GetUserByInviteToken(r.Context(), req.Token)
	if err != nil {
		http.Error(w, `{"error":"invalid or expired invite token"}`, http.StatusNotFound)
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	activated, err := h.repo.ActivateUser(r.Context(), user.ID, string(hash))
	if err != nil {
		http.Error(w, `{"error":"activation failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "activated",
		"user":   activated,
	})
}

func generateInviteToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
