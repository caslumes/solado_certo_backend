package httpme

import (
	"encoding/json"
	"net/http"

	httpconstants "github.com/caslumes/solado_certo_backend/internal/app/constants/http"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
)

type editProfileRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
	AvatarURL string `json:"avatar_url"`
}

type Handler struct {
	getProfile me.GetProfileUseCase
	editProfile me.EditProfileUseCase
}

func NewHandler(getProfile me.GetProfileUseCase, editProfile me.EditProfileUseCase) *Handler {
	return &Handler{getProfile: getProfile, editProfile: editProfile}
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

func (h *Handler) EditProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)

	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		http.Error(w, "could not get user ID", http.StatusUnauthorized)
		return
	}

	var request editProfileRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, "could not decode input", http.StatusBadRequest)
		return
	}

	input := me.ProfileInput{
		Name:  request.Name,
		Email: request.Email,
		Phone: request.Phone,
		AvatarURL: request.AvatarURL,
	}

	profile, err := h.editProfile.EditProfile(r.Context(), userID, input)
	if err != nil {
		http.Error(w, "could not edit profile", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(profile.ToOutput())
}
