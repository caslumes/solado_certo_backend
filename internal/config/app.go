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
	httpfiles "github.com/caslumes/solado_certo_backend/internal/features/files/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	meRepository "github.com/caslumes/solado_certo_backend/internal/features/me/repository/postgres"
	httpme "github.com/caslumes/solado_certo_backend/internal/features/me/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/seller"
	sellerRepository "github.com/caslumes/solado_certo_backend/internal/features/seller/repository/postgres"
	httpseller "github.com/caslumes/solado_certo_backend/internal/features/seller/transport/http"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	rolemiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/role"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

type App struct {
	Envs           *Envs
	DB             *pgxpool.Pool
	AuthMiddleware *authmiddleware.AuthMiddleware
	RoleMiddleware *rolemiddleware.RoleMiddleware
	AuthHandler    *httpauth.Handler
	MeHandler      *httpme.Handler
	CatalogHandler *httpcatalog.Handler
	FilesHandler   *httpfiles.Handler
	SellerHandler  *httpseller.Handler
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

	fileStorage, err := storage.NewLocalStorage(envs.UploadsDir)
	if err != nil {
		return nil, err
	}

	authService := auth.NewService(authRepository.NewPostgresRepository(dbPool), envs.SecretKey)
	meService := me.NewService(meRepository.NewPostgresRepository(dbPool), fileStorage)
	sellerService := seller.NewService(sellerRepository.NewPostgresRepository(dbPool), fileStorage)
	catalogService := catalog.NewService(catalogRepository.NewPostgresRepository(dbPool), meService)

	return &App{
		Envs:           envs,
		DB:             dbPool,
		AuthMiddleware: authmiddleware.NewAuthMiddleware(authService),
		RoleMiddleware: rolemiddleware.NewRoleMiddleware(meService),
		AuthHandler:    httpauth.NewHandler(authService),
		MeHandler:      httpme.NewHandler(meService),
		CatalogHandler: httpcatalog.NewHandler(catalogService),
		FilesHandler:   httpfiles.NewHandler(fileStorage),
		SellerHandler:  httpseller.NewHandler(sellerService),
	}, nil
}
