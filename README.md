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
| `SERVER_PORT` | No (default `8080`) | HTTP port |
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `SECRET_KEY` | Yes | JWT signing key, at least 32 bytes |
