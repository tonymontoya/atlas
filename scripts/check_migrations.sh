#!/usr/bin/env sh
set -eu

root="${1:-db/migrations}"

for up in "$root"/*.up.sql; do
    down="${up%.up.sql}.down.sql"
    if [ ! -f "$down" ]; then
        echo "missing down migration for $up" >&2
        exit 1
    fi
done

for down in "$root"/*.down.sql; do
    up="${down%.down.sql}.up.sql"
    if [ ! -f "$up" ]; then
        echo "missing up migration for $down" >&2
        exit 1
    fi
done

echo "migration file pairs ok"
