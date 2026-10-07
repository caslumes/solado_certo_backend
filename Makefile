SERVER_CONTAINER=server
DB_CONTAINER=db
EXEC=docker compose exec
DUMP_FILE=data/dump.sql
SEED_FILE=data/seeds/dev.sql
DB_USER?=postgres
DB_NAME?=solado_certo

.PHONY: up down dump restore migrate seed restart logs test format secret-key tidy audit

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
	${EXEC} ${SERVER_CONTAINER} go test ./...
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