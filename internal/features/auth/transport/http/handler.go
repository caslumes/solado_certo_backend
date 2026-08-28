package httpauth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/features/auth"
)

type Handler struct {
	register auth.RegisterUseCase
}

func NewHandler(register auth.RegisterUseCase) *Handler {
	return &Handler{register: register}
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
		if errors.Is(err, auth.ErrInvalidInput) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		http.Error(w, "could not register user", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
