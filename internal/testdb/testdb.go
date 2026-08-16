// Package testdb provides the shared PostgreSQL integration-test harness:
// open-or-skip connectivity against ATLAS_TEST_DATABASE_URL plus scoped
// cleanup helpers that encode the schema's foreign-key deletion order in
// one place. Test-only; production code must not import it.
package testdb

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// URL returns ATLAS_TEST_DATABASE_URL, skipping the test when unset.
func URL(t testing.TB) string {
	t.Helper()
	databaseURL := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ATLAS_TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	return databaseURL
}

// Open connects to the test database, verifies connectivity, and closes
// the handle when the test ends. It also returns the URL so API-level
// tests can plug it into app.NewFromConfig.
func Open(t testing.TB) (*sql.DB, string) {
	t.Helper()
	databaseURL := URL(t)
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		t.Fatalf("ping postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, databaseURL
}

func exec(t testing.TB, db *sql.DB, statement string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), statement, args...); err != nil {
		t.Fatalf("exec %q: %v", statement, err)
	}
}

// DeleteSyncRuns deletes inventory sync runs matching the where clause,
// a SQL boolean expression such as "provider = $1". Sync runs are a leaf
// table; DeleteClusters already clears runs that reference matching
// clusters, so this is only needed for extra scoping or orphaned runs.
func DeleteSyncRuns(t testing.TB, db *sql.DB, where string, args ...any) {
	t.Helper()
	exec(t, db, "DELETE FROM inventory_sync_runs WHERE "+where, args...)
}

// DeleteClusters deletes atlas_clusters rows matching the where clause
// along with every cascaded inventory row (snapshots, observations,
// devices, read models). It first clears the inventory_sync_runs rows
// referencing those clusters' snapshots — the one inventory edge without
// ON DELETE CASCADE — so callers never need to know the order.
func DeleteClusters(t testing.TB, db *sql.DB, where string, args ...any) {
	t.Helper()
	exec(t, db, "DELETE FROM inventory_sync_runs WHERE snapshot_id IN (SELECT id FROM inventory_snapshots WHERE cluster_id IN (SELECT id FROM atlas_clusters WHERE "+where+"))", args...)
	exec(t, db, "DELETE FROM atlas_clusters WHERE "+where, args...)
}

// DeleteCases deletes cases matching the where clause along with every
// dependent row. case_alert_dedup is the one case edge without ON DELETE
// CASCADE, so it is cleared first; notes, timeline events, and workflow
// instances (with their jobs, approvals, and task completions) cascade.
func DeleteCases(t testing.TB, db *sql.DB, where string, args ...any) {
	t.Helper()
	exec(t, db, "DELETE FROM case_alert_dedup WHERE case_id IN (SELECT id FROM cases WHERE "+where+")", args...)
	exec(t, db, "DELETE FROM cases WHERE "+where, args...)
}

// DeleteAlertRuns deletes alert_evaluation_runs rows matching the where
// clause, a SQL boolean expression such as "provider = 'fake'".
func DeleteAlertRuns(t testing.TB, db *sql.DB, where string, args ...any) {
	t.Helper()
	exec(t, db, "DELETE FROM alert_evaluation_runs WHERE "+where, args...)
}
