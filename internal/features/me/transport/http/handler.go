package httpme

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app"
	httpconstants "github.com/caslumes/solado_certo_backend/internal/app/constants/http"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
)

type editProfileRequest struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	AvatarURL string `json:"avatar_url"`
}

type addressRequest struct {
	Label      *string `json:"label"`
	Receiver   string  `json:"receiver"`
	ZIPCode    string  `json:"zip_code"`
	Street     string  `json:"street"`
	Number     string  `json:"number"`
	Complement *string `json:"complement"`
	District   string  `json:"district"`
	City       string  `json:"city"`
	State      string  `json:"state"`
	Country    string  `json:"country"`
	IsDefault  bool    `json:"is_default"`
}

type podologicalProfileRequest struct {
	FootstrikeType    string  `json:"footstrike_type"`
	ClinicalCondition string  `json:"clinical_condition"`
	Obs               *string `json:"obs,omitempty"`
}

type Handler struct {
	getProfile             me.GetProfileUseCase
	editProfile            me.EditProfileUseCase
	getAddresses           me.GetAddressesUseCase
	addAddress             me.AddAddressUseCase
	editAddress            me.EditAddressUseCase
	deleteAddress          me.DeleteAddressUseCase
	getPodologicalProfile  me.GetPodologicalProfileUseCase
	editPodologicalProfile me.EditPodologicalProfileUseCase
}

func NewHandler(
	getProfile me.GetProfileUseCase,
	editProfile me.EditProfileUseCase,
	getAddresses me.GetAddressesUseCase,
	addAddress me.AddAddressUseCase,
	editAddress me.EditAddressUseCase,
	deleteAddress me.DeleteAddressUseCase,
	getPodologicalProfile me.GetPodologicalProfileUseCase,
	editPodologicalProfile me.EditPodologicalProfileUseCase,
) *Handler {
	return &Handler{
		getProfile:             getProfile,
		editProfile:            editProfile,
		getAddresses:           getAddresses,
		addAddress:             addAddress,
		editAddress:            editAddress,
		deleteAddress:          deleteAddress,
		getPodologicalProfile:  getPodologicalProfile,
		editPodologicalProfile: editPodologicalProfile,
	}
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
		Name:      request.Name,
		Email:     request.Email,
		Phone:     request.Phone,
		AvatarURL: request.AvatarURL,
	}

	profile, err := h.editProfile.EditProfile(r.Context(), userID, input)
	if err != nil {
		http.Error(w, "could not edit profile", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(profile.ToOutput())
}

func (h *Handler) GetAddresses(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)

	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		http.Error(w, "could not get user ID", http.StatusUnauthorized)
		return
	}

	addresses, err := h.getAddresses.GetAddressesByUserID(r.Context(), userID)
	if err != nil {
		http.Error(w, "could not get addresses", http.StatusInternalServerError)
		return
	}

	var output me.AddressesOutput
	output = output.FromEntities(addresses)
	json.NewEncoder(w).Encode(output)
}

func (h *Handler) AddAddress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)

	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		http.Error(w, "could not get user ID", http.StatusUnauthorized)
		return
	}

	var request addressRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, "could not decode input", http.StatusBadRequest)
		return
	}

	input := me.AddressInput{
		Label:      request.Label,
		Receiver:   request.Receiver,
		ZIPCode:    request.ZIPCode,
		Street:     request.Street,
		Number:     request.Number,
		Complement: request.Complement,
		District:   request.District,
		City:       request.City,
		State:      request.State,
		Country:    request.Country,
		IsDefault:  request.IsDefault,
	}

	address, err := h.addAddress.AddAddress(r.Context(), userID, input)
	if err != nil {
		http.Error(w, "could not add address", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(address.ToOutput())
}

func (h *Handler) EditAddress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)

	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		http.Error(w, "could not get user ID", http.StatusUnauthorized)
		return
	}

	addressID := r.PathValue("addressID")
	if addressID == "" {
		http.Error(w, "could not get address ID", http.StatusBadRequest)
		return
	}

	var request addressRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, "could not decode input", http.StatusBadRequest)
		return
	}

	input := me.AddressInput{
		Label:      request.Label,
		Receiver:   request.Receiver,
		ZIPCode:    request.ZIPCode,
		Street:     request.Street,
		Number:     request.Number,
		Complement: request.Complement,
		District:   request.District,
		City:       request.City,
		State:      request.State,
		Country:    request.Country,
		IsDefault:  request.IsDefault,
	}

	address, err := h.editAddress.EditAddress(r.Context(), userID, addressID, input)
	if err != nil {
		if errors.Is(err, app.ErrAddressNotFound) {
			http.Error(w, "address not found", http.StatusNotFound)
			return
		}

		http.Error(w, "could not edit address", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(address.ToOutput())
}

func (h *Handler) DeleteAddress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)

	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		http.Error(w, "could not get user ID", http.StatusUnauthorized)
		return
	}

	addressID := r.PathValue("addressID")
	if addressID == "" {
		http.Error(w, "could not get address ID", http.StatusBadRequest)
		return
	}

	err := h.deleteAddress.DeleteAddress(r.Context(), userID, addressID)
	if err != nil {
		if errors.Is(err, app.ErrAddressNotFound) {
			http.Error(w, "address not found", http.StatusNotFound)
			return
		}

		http.Error(w, "could not delete address", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetPodologicalProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)

	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		http.Error(w, "could not get user ID", http.StatusUnauthorized)
		return
	}

	profile, err := h.getPodologicalProfile.GetPodologicalProfileByUserID(r.Context(), userID)
	if err != nil {
		http.Error(w, "could not get podological profile", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(profile.ToOutput())
}

func (h *Handler) EditPodologicalProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)

	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		http.Error(w, "could not get user ID", http.StatusUnauthorized)
		return
	}

	var request podologicalProfileRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, "could not decode input", http.StatusBadRequest)
		return
	}

	input := me.PodologicalProfileInput{
		FootstrikeType:    request.FootstrikeType,
		ClinicalCondition: request.ClinicalCondition,
		Obs:               request.Obs,
	}

	profile, err := h.editPodologicalProfile.EditPodologicalProfile(r.Context(), userID, input)
	if err != nil {
		http.Error(w, "could not edit podological profile", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(profile.ToOutput())
}
