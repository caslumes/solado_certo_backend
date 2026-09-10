package httpme

import (
	"encoding/json"
	"net/http"

	httpconstants "github.com/caslumes/solado_certo_backend/internal/app/constants/http"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
)

type Handler struct {
	getProfile me.GetProfileUseCase
}

func NewHandler(getProfile me.GetProfileUseCase) *Handler {
	return &Handler{getProfile: getProfile}
}

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)

	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		http.Error(w, "could not get user ID", http.StatusUnauthorized)
		return
	}
	profile, err := h.getProfile.GetProfileByUserID(r.Context(), userID)
	if err != nil {
		http.Error(w, "could not get profile", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(profile.ToOutput())
}
