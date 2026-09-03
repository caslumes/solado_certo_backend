package httpauth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app"
	httpconstants "github.com/caslumes/solado_certo_backend/internal/app/constants/http"
	"github.com/caslumes/solado_certo_backend/internal/features/auth"
)

type Handler struct {
	register auth.RegisterUseCase
	login   auth.LoginUseCase
	refreshToken auth.RefreshTokenUseCase
}

func NewHandler(register auth.RegisterUseCase, login auth.LoginUseCase, refreshToken auth.RefreshTokenUseCase) *Handler {
	return &Handler{register: register, login: login, refreshToken: refreshToken}
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	err := h.register.Register(r.Context(), &auth.RegisterInput{
		Name:     request.Name,
		Email:    request.Email,
		Phone:    request.Phone,
		Password: request.Password,
	})
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		http.Error(w, "could not register user", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	tokenPair, err := h.login.Login(r.Context(), &auth.LoginInput{
		Email:    request.Email,
		Password: request.Password,
	})
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, app.ErrInvalidCredentials) {
			http.Error(w, "invalid email or password", http.StatusNotFound)
			return
		}
		http.Error(w, "could not login user", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"access_token": tokenPair.AccessToken, "refresh_token": tokenPair.RefreshToken})
}

func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)
	var request struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if request.RefreshToken == "" {
		http.Error(w, "refresh token is required", http.StatusBadRequest)
		return
	}

	tokenPair, err := h.refreshToken.RefreshToken(r.Context(), request.RefreshToken)
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, app.ErrTokenNotFound) {
			http.Error(w, "refresh token not found or does not match user", http.StatusNotFound)
			return
		}
		if errors.Is(err, app.ErrTokenRevoked) {
			http.Error(w, "refresh token has been revoked", http.StatusUnauthorized)
			return
		}
		http.Error(w, "could not refresh token", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"access_token": tokenPair.AccessToken, "refresh_token": tokenPair.RefreshToken})
}
