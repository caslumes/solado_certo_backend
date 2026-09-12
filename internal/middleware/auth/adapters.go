package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func GinAuthAdapter(a *AuthMiddleware) func(*gin.Context) {
	return func(c *gin.Context) {
		authenticated := false

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Request = r
			authenticated = true
		})

		a.Middleware(next).ServeHTTP(c.Writer, c.Request)

		if authenticated {
			c.Next()
			return
		}

		c.Abort()
	}
}
