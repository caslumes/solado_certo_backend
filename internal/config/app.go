package config

import (
	"github.com/caslumes/solado_certo_backend/internal/features/auth"
)

type App struct {
	Envs 	   *Envs
	RegisterUser auth.RegisterUseCase
}

func InitApp() *App {
	dependencies := InitDependencies()

	return &App{
		Envs:         dependencies.envs,
		RegisterUser: dependencies.registerUser,
	}
}
