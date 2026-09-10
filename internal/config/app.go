package config

import (
	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	"github.com/caslumes/solado_certo_backend/internal/middleware"
)

type App struct {
	Envs 	   *Envs
	AuthMiddleware *middleware.AuthMiddleware
	RegisterUser auth.RegisterUseCase
	LoginUser    auth.LoginUseCase
	Logout       auth.LogoutUseCase
	RefreshToken auth.RefreshTokenUseCase
}

func InitApp() *App {
	dependencies := InitDependencies()

	return &App{
		Envs:         dependencies.envs,
		RegisterUser: dependencies.registerUser,
		LoginUser: dependencies.login,
		Logout: dependencies.logout,
		RefreshToken: dependencies.refreshToken,
	}
}
