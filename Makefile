SERVER_CONTAINER=server
EXEC=docker compose exec

up:
	docker compose up --build -d
down:
	docker compose down
restart: down up
logs:
	docker compose logs -f
format:
	go fmt ./...
tidy:
	go mod tidy
audit:
	go mod verify
	go mod tidy
	go fmt ./...
	${EXEC} ${SERVER_CONTAINER} golangci-lint run