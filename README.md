# Solado Certo Backend

REST API for the Solado Certo app, written in Go with Gin and PostgreSQL.

## Requirements

- Docker with Docker Compose
- Make
- OpenSSL (used by `make secret-key`; included with Git for Windows)

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

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `SERVER_PORT` | No (default `3000`) | HTTP port |
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `SECRET_KEY` | Yes | JWT signing key, at least 32 bytes |

## Database data and switching machines

The Postgres data directory (`data/postgres`) lives inside OneDrive on purpose, so development can continue on another machine with the same data. OneDrive syncs files, not database state, so follow this routine to avoid corrupting it:

1. On the machine you are leaving, run `make down`. It writes a SQL dump to `data/dump.sql` and then stops the containers.
2. Wait until OneDrive shows `data/` as fully synced.
3. On the other machine, wait for OneDrive to finish syncing too, then run `make up`.

Never run the stack on two machines at the same time.

`make dump` writes the dump without stopping anything. If the data directory ever gets corrupted, delete `data/postgres`, run `make up` (Postgres starts with an empty database), then `make restore` to load `data/dump.sql`.

`data/postgres` and `data/dump.sql` are git-ignored: they contain password hashes and users' health data.
