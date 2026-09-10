package config

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	authRepository "github.com/caslumes/solado_certo_backend/internal/features/auth/repository/postgres"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	meRepository "github.com/caslumes/solado_certo_backend/internal/features/me/repository/postgres"
	"github.com/caslumes/solado_certo_backend/internal/middleware/auth"
)

type Dependencies struct {
	envs 	   *Envs
	authMiddleware *middleware.AuthMiddleware
	registerUser auth.RegisterUseCase
	login   auth.LoginUseCase
	logout auth.LogoutUseCase
	refreshToken auth.RefreshTokenUseCase
	getProfile   me.GetProfileUseCase
}

func InitDependencies() *Dependencies {
	envs := LoadEnvs()

	dbPool, err := InitDatabase(envs.DatabaseURL)
	if err != nil {
		panic(err)
	}

	authRepository := authRepository.NewPostgresRepository(dbPool)
	meRepository := meRepository.NewPostgresRepository(dbPool)

	authService := auth.NewService(authRepository, envs.SecretKey)
	meService := me.NewService(meRepository)

	authMiddleware := middleware.NewAuthMiddleware(authService)

	return &Dependencies{
		envs:         envs,
		registerUser: authService,
		login:    authService,
		logout: authService,
		refreshToken: authService,
		authMiddleware: authMiddleware,
		getProfile: meService,
	}
}

func InitDatabase(databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), databaseURL)
	return pool, err
}
