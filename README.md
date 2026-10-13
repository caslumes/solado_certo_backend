# Solado Certo Backend

REST API for Solado Certo, a mobile marketplace where users buy and advertise podiatric products (insoles, orthoses, footwear), with a podological profile for each user. Written in Go with Gin and PostgreSQL. This is an academic prototype (TCC); the Docker Compose setup is for development only.

## Requirements

- Docker with Docker Compose
- Make
- OpenSSL (used by `make secret-key`; included with Git for Windows)
- Go 1.27 (see `.tool-versions`) only if you want to run Go commands outside Docker

## Setup

1. Copy the example environment file:

   ```sh
   cp .env.example .env
   ```

2. Generate a secret key for signing JWTs:

   ```sh
   make secret-key
   ```

   Paste the printed value into `SECRET_KEY` in `.env`. The server refuses to start if `SECRET_KEY` is missing or shorter than 32 bytes.

3. Start the API and the database:

   ```sh
   make up
   ```

   On the first start, Postgres creates the database and runs the migrations (see below). The API is then available at `http://localhost:3000`. Check it with:

   ```sh
   curl http://localhost:3000/health
   ```

   It returns `{"status":"ok"}` once the database is reachable. The first `make up` builds the image and downloads dependencies, so it can take a few minutes.

The server reloads automatically ([Air](https://github.com/air-verse/air)) when Go files change. `make logs` follows the logs.

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `SERVER_PORT` | No (default `3000`) | HTTP port |
| `DATABASE_URL` | Yes | PostgreSQL connection string. Inside Compose the host is `db` |
| `SECRET_KEY` | Yes | JWT signing key, at least 32 bytes |

The server exits with a clear error at startup if a required variable is missing or the database is unreachable.

## Database and migrations

SQL migrations live in `data/migrations`, named `YYYY_MM_DD_description.sql`. Compose mounts that folder as `/docker-entrypoint-initdb.d`, so Postgres runs every file in name order **only when the data directory (`data/postgres`) is empty**, that is, on the first start.

A new migration does not run on an existing database. Apply it with:

```sh
make migrate
```

It runs every file in `data/migrations` in name order against the running database. Migration files must therefore be safe to run more than once (`IF NOT EXISTS`, `ON CONFLICT DO NOTHING`), and a schema change goes in a new file instead of editing an applied one.

To start from scratch instead: `make down`, delete `data/postgres`, `make up` (this erases all data; restore with `make restore` if needed).

The migrations create the schema, the onboarding steps and the roles (`buyer`, `seller`, `admin`) only. `make seed` loads development data (users, categories, ads).

From the host, Postgres listens on port `2345` (user `postgres`, password `postgres`, database `solado_certo`).

## Database data and switching machines

The Postgres data directory (`data/postgres`) lives inside OneDrive on purpose, so development can continue on another machine with the same data. OneDrive syncs files, not database state, so follow this routine to avoid corrupting it:

1. On the machine you are leaving, run `make down`. It writes a SQL dump to `data/dump.sql` and then stops the containers.
2. Wait until OneDrive shows `data/` as fully synced.
3. On the other machine, wait for OneDrive to finish syncing too, then run `make up`.

Never run the stack on two machines at the same time.

`make dump` writes the dump without stopping anything. If the data directory ever gets corrupted, delete `data/postgres`, run `make up` (Postgres starts with an empty database), then `make restore` to load `data/dump.sql`.

`data/postgres` and `data/dump.sql` are git-ignored: they contain password hashes and users' health data.

## Make targets

| Target | What it does |
|---|---|
| `make up` | Build and start the API and the database in the background |
| `make down` | Dump the database to `data/dump.sql`, then stop the containers |
| `make restart` | `down` then `up` |
| `make logs` | Follow the container logs |
| `make test` | Run the Go tests inside the server container |
| `make test-db` | Create the `solado_certo_test` database and apply the migrations to it (safe to run again) |
| `make coverage` | Run the tests on the host with a coverage profile and print the total for `internal/` |
| `make dump` | Write a SQL dump to `data/dump.sql` |
| `make restore` | Load `data/dump.sql` into the database |
| `make migrate` | Apply every migration in `data/migrations` to the running database |
| `make seed` | Load the development data in `data/seeds/dev.sql` (safe to run again) |
| `make secret-key` | Print a random value for `SECRET_KEY` |
| `make format` / `make tidy` | `go fmt` / `go mod tidy` (need Go on the host) |
| `make audit` | Verify modules, tidy, format, and run `golangci-lint` in the container |

## API

All bodies are JSON. Errors are returned as `{"error": "message"}`. Routes under `/me` need an `Authorization: Bearer <access_token>` header.

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Liveness and database check |
| `POST` | `/auth/register` | Create an account (`name`, `email`, `phone`, `password`) with the `buyer` role. Rate limited: 5/min per IP |
| `POST` | `/auth/login` | Returns `access_token` and `refresh_token`. Rate limited: 5/min per IP |
| `POST` | `/auth/refresh-token` | Exchange a refresh token for a new token pair (the old one is revoked) |
| `POST` | `/auth/logout` | Revoke a refresh token |
| `GET` / `PATCH` | `/me/profile` | Read / edit name, email, phone, avatar |
| `GET` / `POST` | `/me/addresses` | List / add addresses |
| `PATCH` / `DELETE` | `/me/addresses/:addressID` | Edit / delete an address |
| `GET` / `PUT` | `/me/podological-profile` | Read / save `footstrike_type` (`neutral`, `pronated`, `supinated`, `unknown`), optional `clinical_condition` and `obs`, and `pain_point_ids`. Saving needs an active `podological_profile` consent (403 otherwise) |
| `GET` | `/me/pain-points` | Pain points the user can pick |
| `GET` / `POST` | `/me/consents` | List active consents / grant one (`purpose`, `terms_version`; 201 when new, 200 when already active) |
| `DELETE` | `/me/consents/:purpose` | Revoke a consent. Revoking `podological_profile` also deletes the podological profile |
| `GET` | `/me/roles` | The user's roles: `{"roles": ["buyer", "seller"]}` |
| `POST` | `/me/roles/seller` | Become a seller (no approval step; 201 when new, 200 when already a seller). Returns the roles |
| `GET` / `PUT` | `/me/onboarding` | Read the current onboarding step / change it (`action`: `advance`, `retreat`, `skip`, `dismiss`, `complete`) |
| `GET` | `/catalog` | List published ads. Query: `page`, `limit` (max 100), `q`, `category`, `price_min`, `price_max`, `prescription_required`, `custom_made_allowed`, `side`, `size`, `material`, `rigidity`, `usage_indication` |
| `GET` | `/catalog/items/:advertisementID` | One ad |
| `GET` | `/catalog/categories` | Categories |

## Project structure

```
cmd/server/            entry point: HTTP server, routes, CORS, graceful shutdown
internal/
  config/              environment variables, database pool, dependency wiring (InitApp)
  features/
    auth/              register, login, refresh, logout (JWT + bcrypt)
    me/                profile, addresses, podological profile, consents, roles, onboarding
    catalog/           ads and categories
      <feature>/       service.go (use cases), entity/input/output types,
                       repository/postgres (SQL), transport/http (Gin handlers)
  middleware/          JWT auth, role checks (RequireRole) and per-IP rate limiting
  app/                 shared errors and enums
  helpers/             small shared helpers
data/migrations/       SQL migrations (run on first start)
.setup/build/          Dockerfiles (dev: Air hot reload; prod: compiled binary)
```

Each feature follows the same layering: the HTTP handler decodes the request and maps errors to status codes, the service holds the business rules and validation, and the repository runs SQL with pgx.

## Tests and lint

```sh
make test     # go test ./... inside the server container
make coverage # go test on the host, prints total coverage of internal/
make audit    # includes golangci-lint
```

With Go installed on the host, `go test ./...` works too.

Repository tests run against a separate `solado_certo_test` database in the same Postgres container. Create it once with `make test-db` (and again after adding a migration). Tests that need it read `TEST_DATABASE_URL` (`make test` and `make coverage` set it) and are skipped when it is unset or the database is unreachable. They refuse to run against a database whose name does not end in `_test`, and they empty its tables between tests.
