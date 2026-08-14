#!/usr/bin/env sh
set -eu

database_url="${DATABASE_URL:-postgres://atlas:atlas_dev@127.0.0.1:15432/atlas?sslmode=disable}"
migrations_dir="${1:-db/migrations}"

psql "$database_url" -v ON_ERROR_STOP=1 -c "
CREATE TABLE IF NOT EXISTS atlas_schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
" >/dev/null

for migration in "$migrations_dir"/*.up.sql; do
    version="$(basename "$migration" .up.sql)"
    applied="$(psql "$database_url" -At -v ON_ERROR_STOP=1 -c "SELECT 1 FROM atlas_schema_migrations WHERE version = '$version';")"
    if [ "$applied" = "1" ]; then
        echo "skipping $migration"
        continue
    fi

    echo "applying $migration"
    psql "$database_url" -v ON_ERROR_STOP=1 -f "$migration"
    psql "$database_url" -v ON_ERROR_STOP=1 -c "INSERT INTO atlas_schema_migrations (version) VALUES ('$version');" >/dev/null
done
