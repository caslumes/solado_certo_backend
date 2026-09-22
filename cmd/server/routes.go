package main

import (
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/config"
	httpauth "github.com/caslumes/solado_certo_backend/internal/features/auth/transport/http"
	httpme "github.com/caslumes/solado_certo_backend/internal/features/me/transport/http"
	middleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
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

	authGroup := r.Group("/auth")
	{
		authGroup.POST("/register", gin.WrapH(http.HandlerFunc(authHandler.Register)))
		authGroup.POST("/login", gin.WrapH(http.HandlerFunc(authHandler.Login)))
		authGroup.POST("/logout", gin.WrapH(http.HandlerFunc(authHandler.Logout)))
		authGroup.POST("/refresh-token", gin.WrapH(http.HandlerFunc(authHandler.RefreshToken)))
	}

	meGroup := r.Group("/me")
	meGroup.Use(middleware.GinAuthAdapter(a.AuthMiddleware))
	{
		meGroup.GET("/profile", gin.WrapH(http.HandlerFunc(meHandler.GetProfile)))
		meGroup.PATCH("/profile", gin.WrapH(http.HandlerFunc(meHandler.EditProfile)))

		meGroup.GET("/addresses", gin.WrapH(http.HandlerFunc(meHandler.GetAddresses)))
		meGroup.POST("/addresses", gin.WrapH(http.HandlerFunc(meHandler.AddAddress)))
		meGroup.PATCH("/addresses/:addressID", func(c *gin.Context) {
			c.Request.SetPathValue("addressID", c.Param("addressID"))
			meHandler.EditAddress(c.Writer, c.Request)
		})
		meGroup.DELETE("/addresses/:addressID", func(c *gin.Context) {
			c.Request.SetPathValue("addressID", c.Param("addressID"))
			meHandler.DeleteAddress(c.Writer, c.Request)
		})

		meGroup.GET("/podological-profile", gin.WrapH(http.HandlerFunc(meHandler.GetPodologicalProfile)))
		meGroup.PUT("/podological-profile", gin.WrapH(http.HandlerFunc(meHandler.EditPodologicalProfile)))

		meGroup.GET("/onboarding", gin.WrapH(http.HandlerFunc(meHandler.GetOnboarding)))
		meGroup.PUT("/onboarding", gin.WrapH(http.HandlerFunc(meHandler.ChangeOnboardingStep)))
	}
}
