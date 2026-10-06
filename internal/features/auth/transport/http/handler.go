package httpauth

import (
	"errors"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	register     auth.RegisterUseCase
	login        auth.LoginUseCase
	logout       auth.LogoutUseCase
	refreshToken auth.RefreshTokenUseCase
}

func NewHandler(
	register auth.RegisterUseCase,
	login auth.LoginUseCase,
	logout auth.LogoutUseCase,
	refreshToken auth.RefreshTokenUseCase,
) *Handler {
	return &Handler{register: register, login: login, refreshToken: refreshToken, logout: logout}
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *Handler) Register(c *gin.Context) {
	var request registerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.register.Register(c.Request.Context(), &auth.RegisterInput{
		Name:     request.Name,
		Email:    request.Email,
		Phone:    request.Phone,
		Password: request.Password,
	})
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not register user"})
		return
	}

	c.Status(http.StatusCreated)
}

func (h *Handler) Login(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	tokenPair, err := h.login.Login(c.Request.Context(), &auth.LoginInput{
		Email:    request.Email,
		Password: request.Password,
	})
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, app.ErrInvalidCredentials) {
			c.JSON(http.StatusNotFound, gin.H{"error": "invalid email or password"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not login user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"access_token": tokenPair.AccessToken, "refresh_token": tokenPair.RefreshToken})
}

func (h *Handler) Logout(c *gin.Context) {
	var request refreshTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh token is required"})
		return
	}

	err := h.logout.Logout(c.Request.Context(), request.RefreshToken)
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, app.ErrInvalidToken) || errors.Is(err, app.ErrTokenExpired) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
			return
		}
		if errors.Is(err, app.ErrTokenNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "refresh token not found or does not match user"})
			return
		}
		if errors.Is(err, app.ErrTokenRevoked) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token has already been revoked"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not revoke refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "refresh token revoked successfully"})
}

func (h *Handler) RefreshToken(c *gin.Context) {
	var request refreshTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh token is required"})
		return
	}

	tokenPair, err := h.refreshToken.RefreshToken(c.Request.Context(), request.RefreshToken)
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, app.ErrInvalidToken) || errors.Is(err, app.ErrTokenExpired) || errors.Is(err, app.ErrTokenNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
			return
		}
		if errors.Is(err, app.ErrTokenRevoked) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token has been revoked"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"access_token": tokenPair.AccessToken, "refresh_token": tokenPair.RefreshToken})
}
