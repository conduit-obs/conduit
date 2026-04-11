package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SetupTestDB creates a connection pool to the test database.
// Requires TEST_DATABASE_URL environment variable.
func SetupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("pinging test database: %v", err)
	}

	t.Cleanup(func() { pool.Close() })
	return pool
}

// CreateTestTenant creates a test tenant and returns its ID.
func CreateTestTenant(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name) VALUES ($1) RETURNING id`, name).Scan(&id)
	if err != nil {
		t.Fatalf("creating test tenant: %v", err)
	}
	return id
}

// CreateTestAgent creates a test agent and returns its ID.
func CreateTestAgent(t *testing.T, pool *pgxpool.Pool, tenantID, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO agents (tenant_id, name, labels) VALUES ($1, $2, '{}') RETURNING id`,
		tenantID, name).Scan(&id)
	if err != nil {
		t.Fatalf("creating test agent: %v", err)
	}
	return id
}

// CreateTestFleet creates a test fleet and returns its ID.
func CreateTestFleet(t *testing.T, pool *pgxpool.Pool, tenantID, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO fleets (tenant_id, name, selector) VALUES ($1, $2, '{}') RETURNING id`,
		tenantID, name).Scan(&id)
	if err != nil {
		t.Fatalf("creating test fleet: %v", err)
	}
	return id
}

// CleanupTestData removes all test data from the database.
func CleanupTestData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	tables := []string{"rollout_agents", "rollout_history", "rollouts", "agent_config_history",
		"webhooks", "api_keys", "config_intents", "fleets", "agents", "events",
		"pipeline_templates", "policy_packs", "feature_flags"}
	for _, table := range tables {
		pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s", table))
	}
	pool.Exec(ctx, "DELETE FROM tenants")
}
