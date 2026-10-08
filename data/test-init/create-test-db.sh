#!/bin/sh
set -e

DB_USER="${DB_USER:-postgres}"
TEST_DB_NAME="${TEST_DB_NAME:-solado_certo_test}"

case "$TEST_DB_NAME" in
  *_test) ;;
  *) echo "TEST_DB_NAME must end with _test" >&2; exit 1 ;;
esac

if ! psql -U "$DB_USER" -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = '$TEST_DB_NAME'" | grep -q 1; then
  psql -v ON_ERROR_STOP=1 -q -U "$DB_USER" -d postgres -c "CREATE DATABASE \"$TEST_DB_NAME\""
  echo "created $TEST_DB_NAME"
fi

for f in /docker-entrypoint-initdb.d/*.sql; do
  echo "$f"
  PGOPTIONS="-c client_min_messages=warning" psql -v ON_ERROR_STOP=1 -q -o /dev/null -U "$DB_USER" -d "$TEST_DB_NAME" -f "$f"
done
