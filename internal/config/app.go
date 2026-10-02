package config

import (
	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
)

type App struct {
	Envs                   *Envs
	AuthMiddleware         *authmiddleware.AuthMiddleware
	RegisterUser           auth.RegisterUseCase
	LoginUser              auth.LoginUseCase
	Logout                 auth.LogoutUseCase
	RefreshToken           auth.RefreshTokenUseCase
	GetProfile             me.GetProfileUseCase
	EditProfile            me.EditProfileUseCase
	GetAddresses           me.GetAddressesUseCase
	AddAddress             me.AddAddressUseCase
	EditAddress            me.EditAddressUseCase
	DeleteAddress          me.DeleteAddressUseCase
	GetPodologicalProfile  me.GetPodologicalProfileUseCase
	EditPodologicalProfile me.EditPodologicalProfileUseCase
	GetOnboarding          me.GetOnboardingUseCase
	ChangeOnboardingStep   me.ChangeOnboardingStepUseCase
	GetAdvertisements      catalog.GetAdvertisementsUseCase
}

func InitApp() *App {
	dependencies := InitDependencies()

	return &App{
		Envs:                   dependencies.envs,
		AuthMiddleware:         dependencies.authMiddleware,
		RegisterUser:           dependencies.registerUser,
		LoginUser:              dependencies.login,
		Logout:                 dependencies.logout,
		RefreshToken:           dependencies.refreshToken,
		GetProfile:             dependencies.getProfile,
		EditProfile:            dependencies.editProfile,
		GetAddresses:           dependencies.getAddresses,
		AddAddress:             dependencies.addAddress,
		EditAddress:            dependencies.editAddress,
		DeleteAddress:          dependencies.deleteAddress,
		GetPodologicalProfile:  dependencies.getPodologicalProfile,
		EditPodologicalProfile: dependencies.editPodologicalProfile,
		GetOnboarding:          dependencies.getOnboarding,
		ChangeOnboardingStep:   dependencies.changeOnboardingStep,
		GetAdvertisements:      dependencies.getAdvertisements,
	}
}
