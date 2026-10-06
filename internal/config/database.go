package config

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func InitDatabase(databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), databaseURL)
	return pool, err
}
