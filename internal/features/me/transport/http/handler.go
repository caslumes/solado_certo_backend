package httpme

import (
	"errors"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
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

func (r addressRequest) toInput() me.AddressInput {
	return me.AddressInput{
		Label:      r.Label,
		Receiver:   r.Receiver,
		ZIPCode:    r.ZIPCode,
		Street:     r.Street,
		Number:     r.Number,
		Complement: r.Complement,
		District:   r.District,
		City:       r.City,
		State:      r.State,
		Country:    r.Country,
		IsDefault:  r.IsDefault,
	}
}

type podologicalProfileRequest struct {
	FootstrikeType    string  `json:"footstrike_type"`
	ClinicalCondition string  `json:"clinical_condition"`
	Obs               *string `json:"obs,omitempty"`
}

type onboardingRequest struct {
	Step   string `json:"step"`
	Action string `json:"action"`
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
	getOnboarding          me.GetOnboardingUseCase
	changeOnboardingStep   me.ChangeOnboardingStepUseCase
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
	getOnboarding me.GetOnboardingUseCase,
	changeOnboardingStep me.ChangeOnboardingStepUseCase,
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
		getOnboarding:          getOnboarding,
		changeOnboardingStep:   changeOnboardingStep,
	}
}

func requireUserID(c *gin.Context) (string, bool) {
	id, ok := authmiddleware.UserIDFrom(c.Request.Context())
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "could not get user ID"})
	}
	return id, ok
}

func (h *Handler) GetProfile(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	profile, err := h.getProfile.GetProfileByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get profile"})
		return
	}

	c.JSON(http.StatusOK, profile.ToOutput())
}

func (h *Handler) EditProfile(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request editProfileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	input := me.ProfileInput{
		Name:      request.Name,
		Email:     request.Email,
		Phone:     request.Phone,
		AvatarURL: request.AvatarURL,
	}

	profile, err := h.editProfile.EditProfile(c.Request.Context(), userID, input)
	if errors.Is(err, app.ErrInvalidInput) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name, email or phone"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not edit profile"})
		return
	}

	c.JSON(http.StatusOK, profile.ToOutput())
}

func (h *Handler) GetAddresses(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	addresses, err := h.getAddresses.GetAddressesByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get addresses"})
		return
	}

	var output me.AddressesOutput
	output = output.FromEntities(addresses)
	c.JSON(http.StatusOK, output)
}

func (h *Handler) AddAddress(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request addressRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	address, err := h.addAddress.AddAddress(c.Request.Context(), userID, request.toInput())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not add address"})
		return
	}

	c.JSON(http.StatusOK, address.ToOutput())
}

func (h *Handler) EditAddress(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request addressRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	address, err := h.editAddress.EditAddress(c.Request.Context(), userID, c.Param("addressID"), request.toInput())
	if err != nil {
		if errors.Is(err, app.ErrAddressNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "address not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not edit address"})
		return
	}

	c.JSON(http.StatusOK, address.ToOutput())
}

func (h *Handler) DeleteAddress(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	err := h.deleteAddress.DeleteAddress(c.Request.Context(), userID, c.Param("addressID"))
	if err != nil {
		if errors.Is(err, app.ErrAddressNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "address not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete address"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) GetPodologicalProfile(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	profile, err := h.getPodologicalProfile.GetPodologicalProfileByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get podological profile"})
		return
	}

	c.JSON(http.StatusOK, profile.ToOutput())
}

func (h *Handler) EditPodologicalProfile(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request podologicalProfileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	input := me.PodologicalProfileInput{
		FootstrikeType:    request.FootstrikeType,
		ClinicalCondition: request.ClinicalCondition,
		Obs:               request.Obs,
	}

	profile, err := h.editPodologicalProfile.EditPodologicalProfile(c.Request.Context(), userID, input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not edit podological profile"})
		return
	}

	c.JSON(http.StatusOK, profile.ToOutput())
}

func (h *Handler) GetOnboarding(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	onboarding, err := h.getOnboarding.GetOnboardingByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get onboarding"})
		return
	}

	c.JSON(http.StatusOK, onboarding.ToOutput())
}

func (h *Handler) ChangeOnboardingStep(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request onboardingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	input := me.OnboardingInput{
		Step:   request.Step,
		Action: request.Action,
	}

	err := h.changeOnboardingStep.ChangeOnboardingStep(c.Request.Context(), userID, input)
	if err != nil {
		if errors.Is(err, app.ErrOnboardingNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "onboarding not found"})
			return
		}
		if errors.Is(err, app.ErrOnboardingStepNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "onboarding step not found"})
			return
		}
		if errors.Is(err, app.ErrOnboardingAlreadyCompleted) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "onboarding already completed"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not change onboarding step"})
		return
	}

	c.Status(http.StatusNoContent)
}
