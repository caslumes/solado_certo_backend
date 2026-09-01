package main

import (
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/config"
	httpauth "github.com/caslumes/solado_certo_backend/internal/features/auth/transport/http"
	"github.com/gin-gonic/gin"
)

func initRoutes(r *gin.Engine, a *config.App) {
	authHandler := httpauth.NewHandler(a.RegisterUser, a.LoginUser)

	authGroup := r.Group("/auth")
	{
		authGroup.POST("/register", gin.WrapH(http.HandlerFunc(authHandler.Register)))
		authGroup.POST("/login", gin.WrapH(http.HandlerFunc(authHandler.Login)))
	}
}
