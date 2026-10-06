package main

import (
	"time"

	"github.com/caslumes/solado_certo_backend/internal/config"
	httpauth "github.com/caslumes/solado_certo_backend/internal/features/auth/transport/http"
	httpcatalog "github.com/caslumes/solado_certo_backend/internal/features/catalog/transport/http"
	httpme "github.com/caslumes/solado_certo_backend/internal/features/me/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/middleware/ratelimit"
	"github.com/gin-gonic/gin"
)

func initRoutes(r *gin.Engine, a *config.App) {
	authHandler := httpauth.NewHandler(a.RegisterUser, a.LoginUser, a.Logout, a.RefreshToken)
	meHandler := httpme.NewHandler(
		a.GetProfile,
		a.EditProfile,
		a.GetAddresses,
		a.AddAddress,
		a.EditAddress,
		a.DeleteAddress,
		a.GetPodologicalProfile,
		a.EditPodologicalProfile,
		a.GetOnboarding,
		a.ChangeOnboardingStep,
	)
	catalogHandler := httpcatalog.NewHandler(a.GetAdvertisements, a.GetAdvertisement, a.GetCategories)

	registerLimiter := ratelimit.New(5, time.Minute)
	loginLimiter := ratelimit.New(5, time.Minute)

	authGroup := r.Group("/auth")
	{
		authGroup.POST("/register", registerLimiter.Middleware(), authHandler.Register)
		authGroup.POST("/login", loginLimiter.Middleware(), authHandler.Login)
		authGroup.POST("/logout", authHandler.Logout)
		authGroup.POST("/refresh-token", authHandler.RefreshToken)
	}

	meGroup := r.Group("/me", a.AuthMiddleware.Middleware())
	{
		meGroup.GET("/profile", meHandler.GetProfile)
		meGroup.PATCH("/profile", meHandler.EditProfile)

		meGroup.GET("/addresses", meHandler.GetAddresses)
		meGroup.POST("/addresses", meHandler.AddAddress)
		meGroup.PATCH("/addresses/:addressID", meHandler.EditAddress)
		meGroup.DELETE("/addresses/:addressID", meHandler.DeleteAddress)

		meGroup.GET("/podological-profile", meHandler.GetPodologicalProfile)
		meGroup.PUT("/podological-profile", meHandler.EditPodologicalProfile)

		meGroup.GET("/onboarding", meHandler.GetOnboarding)
		meGroup.PUT("/onboarding", meHandler.ChangeOnboardingStep)
	}

	catalogGroup := r.Group("/catalog")
	{
		catalogGroup.GET("", catalogHandler.GetAdvertisements)
		catalogGroup.GET("/items/:advertisementID", catalogHandler.GetAdvertisement)
		catalogGroup.GET("/categories", catalogHandler.GetCategories)
	}
}
