#!/usr/bin/env bash
set -euo pipefail
# Last upstream revision with paired golang-migrate files; the legacy history
# ends at its final version, later versions exist only in goose format.
reference=58fbf14f93a98c32fb33d8bb99d96dfab20de531
final=49
here=$(cd "$(dirname "$0")" && pwd)
root=$(git rev-parse --show-toplevel)
work=$(mktemp -d)
container="okrs-migration-reference-$$"
cleanup() { docker rm -f "$container" >/dev/null 2>&1 || true; rm -rf "$work"; }
trap cleanup EXIT
mkdir "$work/migrations"
git -C "$root" archive "$reference" migrations | tar -x -C "$work"
cp "$here/runner.go" "$work/main.go"
cat > "$work/go.mod" <<'MOD'
module migration-reference

go 1.25.0

require github.com/golang-migrate/migrate/v4 v4.17.1
MOD
(cd "$work" && go mod tidy && go build -buildvcs=false -o runner .)
if [ -n "${LEGACY_RUNNER_OUTPUT:-}" ]; then cp "$work/runner" "$LEGACY_RUNNER_OUTPUT"; fi
docker run -d --name "$container" -e POSTGRES_PASSWORD=fixture -p 127.0.0.1::5432 postgres:15 >/dev/null
for attempt in {1..60}; do
 if docker exec "$container" pg_isready -h 127.0.0.1 -U postgres >/dev/null 2>&1; then break; fi
 sleep 1
done
port=$(docker port "$container" 5432/tcp | cut -d: -f2)
for version in 22 47 48 "$final"; do
 db="fixture$version"
 docker exec "$container" createdb -U postgres "$db"
 "$work/runner" "$work/migrations" "postgres://postgres:fixture@127.0.0.1:$port/$db?sslmode=disable" 22
 docker exec -i "$container" psql -v ON_ERROR_STOP=1 -U postgres "$db" < "$here/seed22.sql" >/dev/null
 "$work/runner" "$work/migrations" "postgres://postgres:fixture@127.0.0.1:$port/$db?sslmode=disable" "$version"
 if [ "$version" -ge 47 ]; then
  docker exec -i "$container" psql -v ON_ERROR_STOP=1 -U postgres "$db" < "$here/seed47.sql" >/dev/null
 fi
 docker exec "$container" pg_dump -U postgres --no-owner --no-privileges "$db" > "$here/v$version.sql"
 docker exec "$container" createdb -U postgres "restore$version"
 docker exec -i "$container" psql -v ON_ERROR_STOP=1 -U postgres "restore$version" < "$here/v$version.sql" >/dev/null
 actual=$(docker exec "$container" psql -U postgres -At "restore$version" -c 'SELECT version FROM schema_migrations WHERE NOT dirty')
 test "$actual" = "$version"
 "$work/runner" "$work/migrations" "postgres://postgres:fixture@127.0.0.1:$port/restore$version?sslmode=disable" "$final"
 docker exec "$container" pg_dump -U postgres --no-owner --no-privileges "restore$version" > "$here/v${version}-to${final}.sql"
done
docker exec "$container" pg_dump -U postgres --schema-only --no-owner --no-privileges --exclude-table=schema_migrations "fixture$final" > "$here/schema$final.sql"
docker exec "$container" psql -U postgres -At -c 'SHOW server_version' > "$here/postgres-version.txt"
