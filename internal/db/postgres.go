package db

import (
	"context"
	"embed"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Connect creates a connection pool to PostgreSQL.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return pool, nil
}

var allMigrationFiles = []string{
	"migrations/001_initial.sql",
	"migrations/002_phase3.sql",
	"migrations/003_phase5.sql",
	"migrations/004_phase6.sql",
	"migrations/005_phase7.sql",
	"migrations/006_phase8.sql",
	"migrations/007_phase10.sql",
	"migrations/008_phase12.sql",
	"migrations/009_gaps.sql",
	"migrations/010_auth.sql",
	"migrations/011_asset_model.sql",
	"migrations/012_auth0.sql",
}

// GetAllMigrationFiles returns the ordered list of all migration files.
func GetAllMigrationFiles() []string {
	return allMigrationFiles
}

// GetMigrationContent returns the SQL content of a migration file.
func GetMigrationContent(filename string) (string, error) {
	data, err := migrations.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("reading migration file %s: %w", filename, err)
	}
	return string(data), nil
}

// Migrate runs all SQL migration files against the database.
func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	// Ensure migration tracking table exists
	_, _ = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		filename TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)

	for i, f := range allMigrationFiles {
		version := i + 1

		// Skip already-applied migrations
		var exists bool
		pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists)
		if exists {
			logger.Info("migration already applied", "file", f)
			continue
		}

		data, err := migrations.ReadFile(f)
		if err != nil {
			return fmt.Errorf("reading migration file %s: %w", f, err)
		}

		_, err = pool.Exec(ctx, string(data))
		if err != nil {
			return fmt.Errorf("executing migration %s: %w", f, err)
		}

		_, err = pool.Exec(ctx, `INSERT INTO schema_migrations (version, filename) VALUES ($1, $2) ON CONFLICT DO NOTHING`, version, f)
		if err != nil {
			return fmt.Errorf("recording migration %s: %w", f, err)
		}

		logger.Info("migration applied", "file", f, "version", version)
	}

	return nil
}

// MigrationStatus returns lists of applied and pending migrations.
func MigrationStatus(ctx context.Context, pool *pgxpool.Pool) (applied []string, pending []string, err error) {
	// Ensure tracking table exists
	_, _ = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		filename TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)

	appliedSet := make(map[int]bool)
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, nil, err
		}
		appliedSet[v] = true
	}

	for i, f := range allMigrationFiles {
		version := i + 1
		if appliedSet[version] {
			applied = append(applied, f)
		} else {
			pending = append(pending, f)
		}
	}

	return applied, pending, nil
}

// MigrateDown rolls back migrations down to the target version.
// This drops the schema_migrations entries; actual rollback SQL is logged.
func MigrateDown(ctx context.Context, pool *pgxpool.Pool, targetVersion int, logger *slog.Logger) error {
	// Ensure tracking table exists
	_, _ = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		filename TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)

	for i := len(allMigrationFiles); i > targetVersion; i-- {
		var exists bool
		pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, i).Scan(&exists)
		if !exists {
			continue
		}

		_, err := pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, i)
		if err != nil {
			return fmt.Errorf("removing migration record v%d: %w", i, err)
		}
		logger.Info("migration record removed", "version", i, "file", allMigrationFiles[i-1])
	}

	return nil
}
