package main

import (
	"time"

	"github.com/caslumes/solado_certo_backend/internal/config"
	"github.com/caslumes/solado_certo_backend/internal/middleware/ratelimit"
	"github.com/gin-gonic/gin"
)

func initRoutes(r *gin.Engine, a *config.App) {
	registerLimiter := ratelimit.New(5, time.Minute)
	loginLimiter := ratelimit.New(5, time.Minute)

	authGroup := r.Group("/auth")
	{
		authGroup.POST("/register", registerLimiter.Middleware(), a.AuthHandler.Register)
		authGroup.POST("/login", loginLimiter.Middleware(), a.AuthHandler.Login)
		authGroup.POST("/logout", a.AuthHandler.Logout)
		authGroup.POST("/refresh-token", a.AuthHandler.RefreshToken)
	}

	meGroup := r.Group("/me", a.AuthMiddleware.Middleware())
	{
		meGroup.GET("/profile", a.MeHandler.GetProfile)
		meGroup.PATCH("/profile", a.MeHandler.EditProfile)

		meGroup.GET("/addresses", a.MeHandler.GetAddresses)
		meGroup.POST("/addresses", a.MeHandler.AddAddress)
		meGroup.PATCH("/addresses/:addressID", a.MeHandler.EditAddress)
		meGroup.DELETE("/addresses/:addressID", a.MeHandler.DeleteAddress)

		meGroup.GET("/podological-profile", a.MeHandler.GetPodologicalProfile)
		meGroup.PUT("/podological-profile", a.MeHandler.EditPodologicalProfile)

		meGroup.GET("/onboarding", a.MeHandler.GetOnboarding)
		meGroup.PUT("/onboarding", a.MeHandler.ChangeOnboardingStep)
	}

	catalogGroup := r.Group("/catalog")
	{
		catalogGroup.GET("", a.CatalogHandler.GetAdvertisements)
		catalogGroup.GET("/items/:advertisementID", a.CatalogHandler.GetAdvertisement)
		catalogGroup.GET("/categories", a.CatalogHandler.GetCategories)
	}
}
