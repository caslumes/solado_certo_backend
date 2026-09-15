package config

import (
	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	middleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
)

type App struct {
	Envs           *Envs
	AuthMiddleware *middleware.AuthMiddleware
	RegisterUser   auth.RegisterUseCase
	LoginUser      auth.LoginUseCase
	Logout         auth.LogoutUseCase
	RefreshToken   auth.RefreshTokenUseCase
	GetProfile     me.GetProfileUseCase
	EditProfile    me.EditProfileUseCase
	GetAddresses   me.GetAddressesUseCase
	AddAddress     me.AddAddressUseCase
}

func InitApp() *App {
	dependencies := InitDependencies()

	return &App{
		Envs:           dependencies.envs,
		AuthMiddleware: dependencies.authMiddleware,
		RegisterUser:   dependencies.registerUser,
		LoginUser:      dependencies.login,
		Logout:         dependencies.logout,
		RefreshToken:   dependencies.refreshToken,
		GetProfile:     dependencies.getProfile,
		EditProfile:    dependencies.editProfile,
		GetAddresses:   dependencies.getAddresses,
		AddAddress:     dependencies.addAddress,
	}
}
