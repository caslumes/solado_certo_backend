package main

import (
	"context"
	"net/http"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/config"
	"github.com/caslumes/solado_certo_backend/internal/middleware/ratelimit"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func initRoutes(r *gin.Engine, a *config.App) {
	r.GET("/health", health(a.DB))

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

func health(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "database unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
