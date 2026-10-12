package config

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	authRepository "github.com/caslumes/solado_certo_backend/internal/features/auth/repository/postgres"
	httpauth "github.com/caslumes/solado_certo_backend/internal/features/auth/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	catalogRepository "github.com/caslumes/solado_certo_backend/internal/features/catalog/repository/postgres"
	httpcatalog "github.com/caslumes/solado_certo_backend/internal/features/catalog/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	meRepository "github.com/caslumes/solado_certo_backend/internal/features/me/repository/postgres"
	httpme "github.com/caslumes/solado_certo_backend/internal/features/me/transport/http"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
)

type App struct {
	Envs           *Envs
	DB             *pgxpool.Pool
	AuthMiddleware *authmiddleware.AuthMiddleware
	AuthHandler    *httpauth.Handler
	MeHandler      *httpme.Handler
	CatalogHandler *httpcatalog.Handler
}

func InitApp(ctx context.Context) (*App, error) {
	envs, err := LoadEnvs()
	if err != nil {
		return nil, err
	}

	dbPool, err := InitDatabase(ctx, envs.DatabaseURL)
	if err != nil {
		return nil, err
	}

	authService := auth.NewService(authRepository.NewPostgresRepository(dbPool), envs.SecretKey)
	meService := me.NewService(meRepository.NewPostgresRepository(dbPool))
	catalogService := catalog.NewService(catalogRepository.NewPostgresRepository(dbPool), meService)

	return &App{
		Envs:           envs,
		DB:             dbPool,
		AuthMiddleware: authmiddleware.NewAuthMiddleware(authService),
		AuthHandler:    httpauth.NewHandler(authService),
		MeHandler:      httpme.NewHandler(meService),
		CatalogHandler: httpcatalog.NewHandler(catalogService),
	}, nil
}
