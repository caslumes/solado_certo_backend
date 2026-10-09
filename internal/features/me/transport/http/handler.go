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
	FootstrikeType    string   `json:"footstrike_type"`
	ClinicalCondition *string  `json:"clinical_condition"`
	Obs               *string  `json:"obs,omitempty"`
	PainPointIDs      []string `json:"pain_point_ids"`
}

type consentRequest struct {
	Purpose      string `json:"purpose"`
	TermsVersion string `json:"terms_version"`
}

type onboardingRequest struct {
	Step   string `json:"step"`
	Action string `json:"action"`
}

type Service interface {
	me.GetProfileUseCase
	me.EditProfileUseCase
	me.GetAddressesUseCase
	me.AddAddressUseCase
	me.EditAddressUseCase
	me.DeleteAddressUseCase
	me.GetPodologicalProfileUseCase
	me.EditPodologicalProfileUseCase
	me.GetPainPointsUseCase
	me.GetConsentsUseCase
	me.GrantConsentUseCase
	me.RevokeConsentUseCase
	me.GetOnboardingUseCase
	me.ChangeOnboardingStepUseCase
}

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
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

	profile, err := h.service.GetProfileByUserID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, app.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

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

	profile, err := h.service.EditProfile(c.Request.Context(), userID, input)
	if errors.Is(err, app.ErrInvalidInput) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name, email or phone"})
		return
	}
	if errors.Is(err, app.ErrUserNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if errors.Is(err, app.ErrUserAlreadyExists) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
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

	addresses, err := h.service.GetAddressesByUserID(c.Request.Context(), userID)
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

	address, err := h.service.AddAddress(c.Request.Context(), userID, request.toInput())
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

	address, err := h.service.EditAddress(c.Request.Context(), userID, c.Param("addressID"), request.toInput())
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

	err := h.service.DeleteAddress(c.Request.Context(), userID, c.Param("addressID"))
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

	profile, err := h.service.GetPodologicalProfileByUserID(c.Request.Context(), userID)
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
		PainPointIDs:      request.PainPointIDs,
	}

	profile, err := h.service.EditPodologicalProfile(c.Request.Context(), userID, input)
	if err != nil {
		if errors.Is(err, app.ErrConsentRequired) {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, app.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not edit podological profile"})
		return
	}

	c.JSON(http.StatusOK, profile.ToOutput())
}

func (h *Handler) GetPainPoints(c *gin.Context) {
	painPoints, err := h.service.GetPainPoints(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get pain points"})
		return
	}

	outputs := make([]me.PainPointOutput, 0, len(painPoints))
	for _, painPoint := range painPoints {
		outputs = append(outputs, *painPoint.ToOutput())
	}

	c.JSON(http.StatusOK, outputs)
}

func (h *Handler) GetConsents(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	consents, err := h.service.GetConsents(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get consents"})
		return
	}

	outputs := make([]me.ConsentOutput, 0, len(consents))
	for _, consent := range consents {
		outputs = append(outputs, *consent.ToOutput())
	}

	c.JSON(http.StatusOK, outputs)
}

func (h *Handler) GrantConsent(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request consentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	consent, created, err := h.service.GrantConsent(c.Request.Context(), userID, me.ConsentInput{
		Purpose:      request.Purpose,
		TermsVersion: request.TermsVersion,
	})
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not grant consent"})
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, consent.ToOutput())
}

func (h *Handler) RevokeConsent(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	err := h.service.RevokeConsent(c.Request.Context(), userID, c.Param("purpose"))
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not revoke consent"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) GetOnboarding(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	onboarding, err := h.service.GetOnboardingByUserID(c.Request.Context(), userID)
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

	err := h.service.ChangeOnboardingStep(c.Request.Context(), userID, input)
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
		if errors.Is(err, app.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid onboarding transition"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not change onboarding step"})
		return
	}

	c.Status(http.StatusNoContent)
}
