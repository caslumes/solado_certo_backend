package httpauth

import (
	"errors"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	"github.com/gin-gonic/gin"
)

type Service interface {
	auth.RegisterUseCase
	auth.LoginUseCase
	auth.LogoutUseCase
	auth.RefreshTokenUseCase
	auth.ForgotPasswordUseCase
	auth.ResetPasswordUseCase
}

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
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

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

func (h *Handler) Register(c *gin.Context) {
	var request registerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.service.Register(c.Request.Context(), &auth.RegisterInput{
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
		if errors.Is(err, app.ErrUserAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
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

	tokenPair, err := h.service.Login(c.Request.Context(), &auth.LoginInput{
		Email:    request.Email,
		Password: request.Password,
	})
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, app.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
			return
		}
		if errors.Is(err, app.ErrAccountBlocked) {
			c.JSON(http.StatusForbidden, gin.H{"error": "this account is blocked", "code": "account_blocked"})
			return
		}
		var suspended *app.AccountSuspendedError
		if errors.As(err, &suspended) {
			c.JSON(http.StatusForbidden, gin.H{"error": "this account is suspended", "code": "account_suspended", "suspended_until": suspended.Until})
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

	err := h.service.Logout(c.Request.Context(), request.RefreshToken)
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

	tokenPair, err := h.service.RefreshToken(c.Request.Context(), request.RefreshToken)
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

func (h *Handler) ForgotPassword(c *gin.Context) {
	var request forgotPasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.service.ForgotPassword(c.Request.Context(), request.Email)
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not send the code"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "if the e-mail is registered, a code was sent to it"})
}

func (h *Handler) ResetPassword(c *gin.Context) {
	var request resetPasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	err := h.service.ResetPassword(c.Request.Context(), &auth.ResetPasswordInput{
		Email:       request.Email,
		Code:        request.Code,
		NewPassword: request.NewPassword,
	})
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) || errors.Is(err, app.ErrInvalidResetCode) || errors.Is(err, app.ErrResetAttemptsExceeded) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not reset the password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password changed"})
}
