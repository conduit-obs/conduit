package db

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
)

var (
	testRepo     *Repo
	testRepoOnce sync.Once
	testRepoErr  error
)

func getTestRepo(t *testing.T) *Repo {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	testRepoOnce.Do(func() {
		ctx := context.Background()
		pool, err := Connect(ctx, dbURL)
		if err != nil {
			testRepoErr = err
			return
		}

		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
		if err := Migrate(ctx, pool, logger); err != nil {
			pool.Close()
			testRepoErr = err
			return
		}

		// Clean tables once
		pool.Exec(ctx, "DELETE FROM events")
		pool.Exec(ctx, "DELETE FROM config_intents")
		pool.Exec(ctx, "DELETE FROM agents")
		pool.Exec(ctx, "DELETE FROM user_roles")
		pool.Exec(ctx, "DELETE FROM roles")
		pool.Exec(ctx, "DELETE FROM tenants")

		testRepo = NewRepo(pool)
	})

	if testRepoErr != nil {
		t.Fatal(testRepoErr)
	}
	return testRepo
}

func TestRepo_TenantCRUD(t *testing.T) {
	repo := getTestRepo(t)
	ctx := context.Background()

	tenant, err := repo.CreateTenant(ctx, "test-org-crud")
	if err != nil {
		t.Fatal(err)
	}
	if tenant.Name != "test-org-crud" {
		t.Errorf("expected test-org-crud, got %s", tenant.Name)
	}

	got, err := repo.GetTenantByID(ctx, tenant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "test-org-crud" {
		t.Errorf("expected test-org-crud, got %s", got.Name)
	}
}

func TestRepo_AgentsCRUD(t *testing.T) {
	repo := getTestRepo(t)
	ctx := context.Background()

	tenant, err := repo.CreateTenant(ctx, "agent-test-org-crud")
	if err != nil {
		t.Fatal(err)
	}

	agent, err := repo.CreateAgent(ctx, tenant.ID, "collector-1", map[string]string{"env": "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "collector-1" {
		t.Errorf("expected collector-1, got %s", agent.Name)
	}

	agents, err := repo.ListAgents(ctx, tenant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}

	err = repo.UpdateAgentHeartbeat(ctx, tenant.ID, "collector-1", "connected")
	if err != nil {
		t.Fatal(err)
	}

	agents, err = repo.ListAgents(ctx, tenant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if agents[0].Status != "connected" {
		t.Errorf("expected connected, got %s", agents[0].Status)
	}
}

func TestRepo_TenantIsolation(t *testing.T) {
	repo := getTestRepo(t)
	ctx := context.Background()

	t1, _ := repo.CreateTenant(ctx, "tenant-iso-a")
	t2, _ := repo.CreateTenant(ctx, "tenant-iso-b")

	repo.CreateAgent(ctx, t1.ID, "agent-iso-a", nil)
	repo.CreateAgent(ctx, t2.ID, "agent-iso-b", nil)

	t1Agents, err := repo.ListAgents(ctx, t1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(t1Agents) != 1 || t1Agents[0].Name != "agent-iso-a" {
		t.Errorf("tenant 1 should only see agent-iso-a, got %d agents", len(t1Agents))
	}

	t2Agents, err := repo.ListAgents(ctx, t2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(t2Agents) != 1 || t2Agents[0].Name != "agent-iso-b" {
		t.Errorf("tenant 2 should only see agent-iso-b, got %d agents", len(t2Agents))
	}
}

func TestRepo_ConfigIntentsCRUD(t *testing.T) {
	repo := getTestRepo(t)
	ctx := context.Background()

	tenant, _ := repo.CreateTenant(ctx, "config-test-org-crud")
	yamlOut := "receivers:\n  otlp:\n"
	intentJSON := `{"version":"1.0","pipelines":[{"name":"test","signal":"traces","receivers":[{"type":"otlp"}],"exporters":[{"type":"debug"}]}]}`

	ci, err := repo.CreateConfigIntent(ctx, tenant.ID, "my-pipeline-crud", intentJSON, &yamlOut)
	if err != nil {
		t.Fatal(err)
	}
	if ci.Name != "my-pipeline-crud" {
		t.Errorf("expected my-pipeline-crud, got %s", ci.Name)
	}
	if ci.Version != 1 {
		t.Errorf("expected version 1, got %d", ci.Version)
	}
	if ci.CompiledYAML == nil || *ci.CompiledYAML != yamlOut {
		t.Error("expected compiled yaml")
	}

	// Creating same name should increment version
	ci2, err := repo.CreateConfigIntent(ctx, tenant.ID, "my-pipeline-crud", intentJSON, &yamlOut)
	if err != nil {
		t.Fatal(err)
	}
	if ci2.Version != 2 {
		t.Errorf("expected version 2, got %d", ci2.Version)
	}

	// Get latest
	got, err := repo.GetConfigIntent(ctx, tenant.ID, "my-pipeline-crud")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 {
		t.Errorf("expected latest version 2, got %d", got.Version)
	}

	// List all
	intents, err := repo.ListConfigIntents(ctx, tenant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 2 {
		t.Errorf("expected 2 intents, got %d", len(intents))
	}
}

func TestRepo_Events(t *testing.T) {
	repo := getTestRepo(t)
	ctx := context.Background()

	tenant, _ := repo.CreateTenant(ctx, "events-test-org-crud")

	err := repo.CreateEvent(ctx, tenant.ID, "agent.connected", map[string]string{"agent": "a1"})
	if err != nil {
		t.Fatal(err)
	}
}
