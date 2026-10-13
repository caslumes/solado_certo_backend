SERVER_CONTAINER=server
DB_CONTAINER=db
EXEC=docker compose exec
DUMP_FILE=data/dump.sql
SEED_FILE=data/seeds/dev.sql
DB_USER?=postgres
DB_NAME?=solado_certo
TEST_DB_NAME?=solado_certo_test
TEST_DATABASE_URL?=postgres://postgres:postgres@localhost:2345/${TEST_DB_NAME}?sslmode=disable
TEST_DATABASE_URL_DOCKER?=postgres://postgres:postgres@db:5432/${TEST_DB_NAME}?sslmode=disable
COVERAGE_FILE=coverage.out

.PHONY: up down dump restore migrate seed restart logs test test-db coverage format secret-key tidy audit

up:
	docker compose up --build -d
down:
	-@$(MAKE) --no-print-directory dump
	docker compose down
dump:
	${EXEC} -T ${DB_CONTAINER} pg_dump --clean --if-exists -U ${DB_USER} -d ${DB_NAME} -f /tmp/dump.sql
	docker compose cp ${DB_CONTAINER}:/tmp/dump.sql ${DUMP_FILE}
restore:
	docker compose cp ${DUMP_FILE} ${DB_CONTAINER}:/tmp/dump.sql
	${EXEC} -T ${DB_CONTAINER} psql -v ON_ERROR_STOP=1 -q -o /dev/null -U ${DB_USER} -d ${DB_NAME} -f /tmp/dump.sql
migrate:
	${EXEC} -T ${DB_CONTAINER} sh -c 'for f in /docker-entrypoint-initdb.d/*.sql; do echo "$$f"; PGOPTIONS="-c client_min_messages=warning" psql -v ON_ERROR_STOP=1 -q -o /dev/null -U ${DB_USER} -d ${DB_NAME} -f "$$f" || exit 1; done'
seed:
	docker compose cp ${SEED_FILE} ${DB_CONTAINER}:/tmp/seed.sql
	${EXEC} -T ${DB_CONTAINER} psql -v ON_ERROR_STOP=1 -q -o /dev/null -U ${DB_USER} -d ${DB_NAME} -f /tmp/seed.sql
restart: down up
logs:
	docker compose logs -f
test:
	${EXEC} -e TEST_DATABASE_URL=${TEST_DATABASE_URL_DOCKER} ${SERVER_CONTAINER} go test ./...
test-db:
	${EXEC} -T -e DB_USER=${DB_USER} -e TEST_DB_NAME=${TEST_DB_NAME} ${DB_CONTAINER} sh -s < data/test-init/create-test-db.sh
coverage:
	TEST_DATABASE_URL='${TEST_DATABASE_URL}' go test ./... -coverpkg=./internal/...,./pkg/... -coverprofile=${COVERAGE_FILE}
	@grep -v '/internal/testutil/' ${COVERAGE_FILE} > ${COVERAGE_FILE}.tmp && mv ${COVERAGE_FILE}.tmp ${COVERAGE_FILE}
	@go tool cover -func=${COVERAGE_FILE} | tail -n 1
format:
	go fmt ./...
secret-key:
	@openssl rand -base64 48
tidy:
	go mod tidy
audit:
	go mod verify
	go mod tidy
	go fmt ./...
	${EXEC} ${SERVER_CONTAINER} golangci-lint run