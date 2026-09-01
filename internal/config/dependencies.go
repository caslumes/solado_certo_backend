package config

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	authRepository "github.com/caslumes/solado_certo_backend/internal/features/auth/repository/postgres"
)

type Dependencies struct {
	envs 	   *Envs
	registerUser auth.RegisterUseCase
	loginUser   auth.LoginUseCase
}

func InitDependencies() *Dependencies {
	envs := LoadEnvs()

	dbPool, err := InitDatabase(envs.DatabaseURL)
	if err != nil {
		panic(err)
	}

	authRepository := authRepository.NewPostgresRepository(dbPool)
	authService := auth.NewService(authRepository)

	return &Dependencies{
		envs:         envs,
		registerUser: authService,
		loginUser:    authService,
	}
}

func InitDatabase(databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), databaseURL)
	return pool, err
}
