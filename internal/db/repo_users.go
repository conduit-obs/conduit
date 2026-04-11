package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// UserRow represents a row in the users table.
type UserRow struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // never expose
	Roles        []string  `json:"roles"`
	Status       string    `json:"status"`
	InviteToken  *string   `json:"invite_token,omitempty"`
	InvitedBy    *string   `json:"invited_by,omitempty"`
	Auth0Sub     *string   `json:"auth0_sub,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateUser creates a new user.
func (r *Repo) CreateUser(ctx context.Context, tenantID, email, passwordHash string, roles []string) (*UserRow, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO users (tenant_id, email, password_hash, roles)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, tenant_id, email, password_hash, roles, status, invite_token, invited_by, auth0_sub, created_at, updated_at`,
		tenantID, email, passwordHash, roles)
	return scanUser(row)
}

// GetUserByEmail looks up a user by email.
func (r *Repo) GetUserByEmail(ctx context.Context, email string) (*UserRow, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, email, password_hash, roles, status, invite_token, invited_by, auth0_sub, created_at, updated_at
		 FROM users WHERE email = $1`, email)
	return scanUser(row)
}

// GetUserByInviteToken looks up a user by invite token.
func (r *Repo) GetUserByInviteToken(ctx context.Context, token string) (*UserRow, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, email, password_hash, roles, status, invite_token, invited_by, auth0_sub, created_at, updated_at
		 FROM users WHERE invite_token = $1 AND status = 'pending'`, token)
	return scanUser(row)
}

// ListUsers returns all users for a tenant.
func (r *Repo) ListUsers(ctx context.Context, tenantID string) ([]UserRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, email, password_hash, roles, status, invite_token, invited_by, auth0_sub, created_at, updated_at
		 FROM users WHERE tenant_id = $1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []UserRow
	for rows.Next() {
		u, err := scanUserRows(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, nil
}

// CreateInvitedUser creates a user with pending status and invite token.
func (r *Repo) CreateInvitedUser(ctx context.Context, tenantID, email, inviteToken, invitedBy string, roles []string) (*UserRow, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO users (tenant_id, email, roles, status, invite_token, invited_by)
		 VALUES ($1, $2, $3, 'pending', $4, $5)
		 RETURNING id, tenant_id, email, password_hash, roles, status, invite_token, invited_by, auth0_sub, created_at, updated_at`,
		tenantID, email, roles, inviteToken, invitedBy)
	return scanUser(row)
}

// ActivateUser sets password and activates a pending user.
func (r *Repo) ActivateUser(ctx context.Context, userID, passwordHash string) (*UserRow, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE users SET password_hash = $1, status = 'active', invite_token = NULL, updated_at = now()
		 WHERE id = $2
		 RETURNING id, tenant_id, email, password_hash, roles, status, invite_token, invited_by, auth0_sub, created_at, updated_at`,
		passwordHash, userID)
	return scanUser(row)
}

// UpdateUser updates roles and/or status.
func (r *Repo) UpdateUser(ctx context.Context, userID string, roles []string, status string) (*UserRow, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE users SET roles = COALESCE($1, roles), status = COALESCE($2, status), updated_at = now()
		 WHERE id = $3
		 RETURNING id, tenant_id, email, password_hash, roles, status, invite_token, invited_by, auth0_sub, created_at, updated_at`,
		roles, status, userID)
	return scanUser(row)
}

// CountTenants returns the number of tenants (used for initialization check).
func (r *Repo) CountTenants(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM tenants`).Scan(&count)
	return count, err
}

func scanUser(row pgx.Row) (*UserRow, error) {
	var u UserRow
	err := row.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.Roles, &u.Status, &u.InviteToken, &u.InvitedBy, &u.Auth0Sub, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning user: %w", err)
	}
	return &u, nil
}

func scanUserRows(rows pgx.Rows) (*UserRow, error) {
	var u UserRow
	err := rows.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.Roles, &u.Status, &u.InviteToken, &u.InvitedBy, &u.Auth0Sub, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning user row: %w", err)
	}
	return &u, nil
}

// CreateUserWithAuth0Sub creates a user linked to an Auth0 account.
func (r *Repo) CreateUserWithAuth0Sub(ctx context.Context, tenantID, email, auth0Sub, name string, roles []string) (*UserRow, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO users (tenant_id, email, auth0_sub, roles, status)
		 VALUES ($1, $2, $3, $4, 'active')
		 RETURNING id, tenant_id, email, password_hash, roles, status, invite_token, invited_by, auth0_sub, created_at, updated_at`,
		tenantID, email, auth0Sub, roles)
	return scanUser(row)
}

// GetUserByAuth0Sub looks up a user by Auth0 subject identifier.
func (r *Repo) GetUserByAuth0Sub(ctx context.Context, auth0Sub string) (*UserRow, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, email, password_hash, roles, status, invite_token, invited_by, auth0_sub, created_at, updated_at
		 FROM users WHERE auth0_sub = $1`, auth0Sub)
	return scanUser(row)
}
