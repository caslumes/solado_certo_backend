package main

import (
	"context"
	"net/http"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app/enum"
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
		meGroup.PUT("/avatar", a.MeHandler.ChangeAvatar)
		meGroup.DELETE("/avatar", a.MeHandler.RemoveAvatar)

		meGroup.GET("/addresses", a.MeHandler.GetAddresses)
		meGroup.POST("/addresses", a.MeHandler.AddAddress)
		meGroup.PATCH("/addresses/:addressID", a.MeHandler.EditAddress)
		meGroup.DELETE("/addresses/:addressID", a.MeHandler.DeleteAddress)

		meGroup.GET("/podological-profile", a.MeHandler.GetPodologicalProfile)
		meGroup.PUT("/podological-profile", a.MeHandler.EditPodologicalProfile)
		meGroup.GET("/pain-points", a.MeHandler.GetPainPoints)

		meGroup.GET("/consents", a.MeHandler.GetConsents)
		meGroup.POST("/consents", a.MeHandler.GrantConsent)
		meGroup.DELETE("/consents/:purpose", a.MeHandler.RevokeConsent)

		meGroup.GET("/roles", a.MeHandler.GetRoles)
		meGroup.POST("/roles/seller", a.MeHandler.BecomeSeller)

		meGroup.GET("/onboarding", a.MeHandler.GetOnboarding)
		meGroup.PUT("/onboarding", a.MeHandler.ChangeOnboardingStep)
	}

	catalogGroup := r.Group("/catalog")
	{
		catalogGroup.GET("", a.AuthMiddleware.OptionalMiddleware(), a.CatalogHandler.GetAdvertisements)
		catalogGroup.GET("/items/:advertisementID", a.CatalogHandler.GetAdvertisement)
		catalogGroup.GET("/categories", a.CatalogHandler.GetCategories)
		catalogGroup.GET("/purposes", a.CatalogHandler.GetPurposes)
		catalogGroup.GET("/conditions", a.CatalogHandler.GetConditions)
	}

	sellerGroup := r.Group("/seller", a.AuthMiddleware.Middleware(), a.RoleMiddleware.RequireRole(enum.RoleSeller))
	{
		sellerGroup.GET("/ads", a.SellerHandler.ListAdvertisements)
		sellerGroup.POST("/ads", a.SellerHandler.CreateAdvertisement)
		sellerGroup.GET("/ads/:advertisementID", a.SellerHandler.GetAdvertisement)
		sellerGroup.PATCH("/ads/:advertisementID", a.SellerHandler.EditAdvertisement)
		sellerGroup.DELETE("/ads/:advertisementID", a.SellerHandler.RemoveAdvertisement)
		sellerGroup.POST("/ads/:advertisementID/publish", a.SellerHandler.PublishAdvertisement)
		sellerGroup.POST("/ads/:advertisementID/pause", a.SellerHandler.PauseAdvertisement)
		sellerGroup.POST("/ads/:advertisementID/reactivate", a.SellerHandler.ReactivateAdvertisement)
		sellerGroup.POST("/ads/:advertisementID/images", a.SellerHandler.AddImage)
		sellerGroup.PATCH("/ads/:advertisementID/images/order", a.SellerHandler.ReorderImages)
		sellerGroup.DELETE("/ads/:advertisementID/images/:imageID", a.SellerHandler.RemoveImage)
	}

	cartGroup := r.Group("/cart", a.AuthMiddleware.Middleware(), a.RoleMiddleware.RequireRole(enum.RoleBuyer))
	{
		cartGroup.GET("", a.OrdersHandler.GetCart)
		cartGroup.POST("/items", a.OrdersHandler.AddCartItem)
		cartGroup.PATCH("/items/:advertisementID", a.OrdersHandler.ChangeCartItemQuantity)
		cartGroup.DELETE("/items/:advertisementID", a.OrdersHandler.RemoveCartItem)
	}

	checkoutGroup := r.Group("/checkout", a.AuthMiddleware.Middleware(), a.RoleMiddleware.RequireRole(enum.RoleBuyer))
	{
		checkoutGroup.POST("", a.OrdersHandler.Checkout)
		checkoutGroup.POST("/preview", a.OrdersHandler.PreviewCheckout)
	}

	r.GET("/files/public/*path", a.FilesHandler.ServePublic)
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
