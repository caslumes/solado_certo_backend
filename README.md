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
| `UPLOADS_DIR` | No (default `data/uploads`) | Folder for uploaded files, with a `public` area (ad photos, avatars) and a `private` one (prescriptions, reports). Git ignores it; like `data/postgres`, it stays in the project folder |
| `PAYMENT_GATEWAY` | No (default `asaas`) | `asaas`, or `fake` to run payments offline: cards are approved except `4000 0000 0000 0002`, and Pix and boleto confirm by themselves after 30 seconds. The fake keeps its charges in memory; after a restart, pending ones are closed and can be paid again |
| `ASAAS_API_KEY` | With `asaas` | Asaas API key (sandbox account) |
| `ASAAS_BASE_URL` | No (default sandbox) | Asaas API address, `https://api-sandbox.asaas.com/v3` by default |
| `ASAAS_WEBHOOK_TOKEN` | No | Token set on the webhook in the Asaas dashboard, checked on `POST /webhooks/asaas`. Without it webhooks are refused and statuses come from polling (every 2 minutes, and whenever the app lists an order's payments) |
| `SMTP_HOST` | No | SMTP server for e-mails (password recovery codes). Compose sets it to `mailpit`; when empty, e-mails are dropped with a warning in the log |
| `SMTP_PORT` | No (default `1025`) | SMTP port |
| `SMTP_USERNAME` / `SMTP_PASSWORD` | No | SMTP login, used only when `SMTP_USERNAME` is set (Go then requires TLS unless the host is `localhost`) |
| `MAIL_FROM` | No (default `Solado Certo <nao-responda@soladocerto.local>`) | Sender of the e-mails |

The server exits with a clear error at startup if a required variable is missing or the database is unreachable.

## E-mail in development

Compose runs [Mailpit](https://mailpit.axllent.org/), which accepts every e-mail on port 1025 and delivers nothing. Open http://localhost:8025 to read what the server sent (such as password recovery codes). A real provider only needs the `SMTP_*` variables.

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

All bodies are JSON. Errors are returned as `{"error": "message"}`. Routes under `/me` and `/seller` need an `Authorization: Bearer <access_token>` header; `/seller` also needs the `seller` role (403 otherwise). A request missing fields needed to save or publish an ad gets 422 with `{"error": "missing required fields", "fields": [...]}` (JSON field names). Uploads accept JPEG, PNG and WebP up to 5 MB; the type is read from the file content, not its name (413 too large, 415 other types).

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Liveness and database check |
| `POST` | `/auth/register` | Create an account (`name`, `email`, `phone`, `password`) with the `buyer` role. Rate limited: 5/min per IP |
| `POST` | `/auth/login` | Returns `access_token` and `refresh_token`. Rate limited: 5/min per IP |
| `POST` | `/auth/refresh-token` | Exchange a refresh token for a new token pair (the old one is revoked) |
| `POST` | `/auth/logout` | Revoke a refresh token |
| `POST` | `/auth/password/forgot` | `email`. Always 202, whether or not the account exists; when it does, e-mails a 6-digit code valid for 15 minutes (at most one per minute, and a new one replaces the previous). Rate limited: 5/min per IP |
| `POST` | `/auth/password/reset` | `email`, `code`, `new_password`. 400 `invalid or expired code`, or `too many attempts, request a new code` after 5 tries of the same code. Success revokes every refresh token of the account. Rate limited: 5/min per IP |
| `GET` / `PATCH` | `/me/profile` | Read / edit name, email and phone (`avatar_url` is ignored; it changes only through `/me/avatar`) |
| `PUT` / `DELETE` | `/me/avatar` | Upload the profile photo (multipart field `avatar`) / remove it. The old file is deleted. Returns the profile |
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
| `GET` | `/seller/ads` | The seller's ads except removed ones, most recently changed first. Optional `status` (`draft`, `published`, `paused`). Each ad has `sold_out` (published with stock 0) and `missing_fields` (what blocks publishing) |
| `POST` | `/seller/ads` | Create a draft. Needs `title` and `category_id`; also `description`, `price` (decimal string), `stock`, `prescription_required`, `custom_made_allowed`, `side` (`left`, `right`, `both`), `size`, `material`, `rigidity` (`flexible`, `semi_rigid`, `rigid`), `usage_indication`, `safety_alerts`, `contraindications`, `shipping_notes`, `purposes` (codes), `condition_ids` and `shipping_options` (`id` to keep an existing one, `name`, `carrier`, `price`, `delivery_days`) |
| `GET` / `PATCH` / `DELETE` | `/seller/ads/:advertisementID` | Read / edit (only the fields sent; `""` clears a text, `[]` a list; a published ad must stay complete) / remove (soft delete). 403 for another seller's ad |
| `POST` | `/seller/ads/:advertisementID/publish` · `/pause` · `/reactivate` | `draft → published`, `published → paused`, `paused → published`. 409 from any other status; publishing an incomplete ad is 422 |
| `POST` | `/seller/ads/:advertisementID/images` | Add a photo (multipart field `image`, optional `alt_text`). At most 6 per ad (409); the first becomes the cover. Returns the ad |
| `PATCH` | `/seller/ads/:advertisementID/images/order` | `image_ids` (every image of the ad, in the new order) and optional `cover_image_id` (keeps the current cover when absent) |
| `DELETE` | `/seller/ads/:advertisementID/images/:imageID` | Remove a photo and its file; the rest are renumbered and the first becomes the cover if the cover was removed. A published ad keeps its last photo (422) |
| `GET` / `POST` | `/support/tickets` | The user's tickets (opened by them, or about an order they bought or sold), most recently changed first; optional `status`, `page`, `limit` / open a ticket: `category` (`order`, `payment`, `adaptation`, `other`), `subject` (5–150 characters), `description` (20–2000) and `order_id` (required for `order` and `payment`; the user must be its buyer or seller). Admins and the other party of the order are notified |
| `GET` | `/support/tickets/:ticketID` | Ticket with its messages (`sender_role`: `buyer`, `seller`, `support` or `user`). Readable by the opener, the order's buyer and seller, and admins; 404 for anyone else |
| `POST` | `/support/tickets/:ticketID/messages` | `message` (up to 2000 characters), as JSON or multipart with an optional `file` (PDF, JPEG or PNG up to 5 MB). An admin's answer moves the ticket to `awaiting_customer`; the customer's answer moves it back. 409 once `resolved` or `closed` |
| `GET` | `/support/tickets/:ticketID/messages/:messageID/attachment` | A message's attachment, for the same readers (reads by others than the sender are audited) |
| `GET` | `/files/public/*path` | A public uploaded file (e.g. `/files/public/ads/<name>.jpg`). Private files are only served by the endpoints that own them |

## Project structure

```
cmd/server/            entry point: HTTP server, routes, CORS, graceful shutdown
internal/
  config/              environment variables, database pool, dependency wiring (InitApp)
  features/
    auth/              register, login, refresh, logout, password recovery (JWT + bcrypt)
    me/                profile, addresses, podological profile, consents, roles, onboarding
    catalog/           ads and categories
    files/             serves public uploaded files
    seller/            seller ads: CRUD and status changes
    support/           support tickets and their messages
      <feature>/       service.go (use cases), entity/input/output types,
                       repository/postgres (SQL), transport/http (Gin handlers)
  middleware/          JWT auth, role checks (RequireRole) and per-IP rate limiting
  app/                 shared errors and enums
  helpers/             small shared helpers
pkg/
  storage/             Storage interface for uploaded files and its local-disk implementation (no dependency on internal/)
  mailer/              Mailer interface, SMTP implementation and a disabled one (no dependency on internal/)
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
