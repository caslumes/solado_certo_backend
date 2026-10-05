package config

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	authRepository "github.com/caslumes/solado_certo_backend/internal/features/auth/repository/postgres"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	catalogRepository "github.com/caslumes/solado_certo_backend/internal/features/catalog/repository/postgres"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	meRepository "github.com/caslumes/solado_certo_backend/internal/features/me/repository/postgres"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
)

type Dependencies struct {
	envs                   *Envs
	authMiddleware         *authmiddleware.AuthMiddleware
	registerUser           auth.RegisterUseCase
	login                  auth.LoginUseCase
	logout                 auth.LogoutUseCase
	refreshToken           auth.RefreshTokenUseCase
	getProfile             me.GetProfileUseCase
	editProfile            me.EditProfileUseCase
	getAddresses           me.GetAddressesUseCase
	addAddress             me.AddAddressUseCase
	editAddress            me.EditAddressUseCase
	deleteAddress          me.DeleteAddressUseCase
	getPodologicalProfile  me.GetPodologicalProfileUseCase
	editPodologicalProfile me.EditPodologicalProfileUseCase
	getOnboarding          me.GetOnboardingUseCase
	changeOnboardingStep   me.ChangeOnboardingStepUseCase
	getAdvertisements      catalog.GetAdvertisementsUseCase
	getAdvertisement       catalog.GetAdvertisementUseCase
	getCategories          catalog.GetCategoriesUseCase
}

func InitDependencies() *Dependencies {
	envs := LoadEnvs()

	dbPool, err := InitDatabase(envs.DatabaseURL)
	if err != nil {
		panic(err)
	}

	authRepository := authRepository.NewPostgresRepository(dbPool)
	meRepository := meRepository.NewPostgresRepository(dbPool)
	catalogRepository := catalogRepository.NewPostgresRepository(dbPool)

	authService := auth.NewService(authRepository, envs.SecretKey)
	meService := me.NewService(meRepository)
	catalogService := catalog.NewService(catalogRepository)

	authMiddleware := authmiddleware.NewAuthMiddleware(authService)

	return &Dependencies{
		envs:                   envs,
		registerUser:           authService,
		login:                  authService,
		logout:                 authService,
		refreshToken:           authService,
		authMiddleware:         authMiddleware,
		getProfile:             meService,
		editProfile:            meService,
		getAddresses:           meService,
		addAddress:             meService,
		editAddress:            meService,
		deleteAddress:          meService,
		getPodologicalProfile:  meService,
		editPodologicalProfile: meService,
		getOnboarding:          meService,
		changeOnboardingStep:   meService,
		getAdvertisements:      catalogService,
		getAdvertisement:       catalogService,
		getCategories:          catalogService,
	}
}

func InitDatabase(databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), databaseURL)
	return pool, err
}
