package main

import (
	"github.com/caslumes/solado_certo_backend/internal/config"
	"github.com/gin-gonic/gin"
)

func main() {
	app := config.InitApp()

	router := gin.Default()

	initRoutes(router, app)

	router.Run(":" + app.Envs.ServerPort)
}
