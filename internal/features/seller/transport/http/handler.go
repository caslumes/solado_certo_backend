package httpseller

import (
	"errors"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/seller"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

type shippingOptionRequest struct {
	ID           *string `json:"id"`
	Name         string  `json:"name"`
	Carrier      *string `json:"carrier"`
	Price        string  `json:"price"`
	DeliveryDays int     `json:"delivery_days"`
}

type advertisementRequest struct {
	CategoryID           *string                  `json:"category_id"`
	Title                *string                  `json:"title"`
	Description          *string                  `json:"description"`
	Price                *string                  `json:"price"`
	Stock                *int                     `json:"stock"`
	PrescriptionRequired *bool                    `json:"prescription_required"`
	CustomMadeAllowed    *bool                    `json:"custom_made_allowed"`
	Side                 *string                  `json:"side"`
	Size                 *string                  `json:"size"`
	Material             *string                  `json:"material"`
	Rigidity             *string                  `json:"rigidity"`
	UsageIndication      *string                  `json:"usage_indication"`
	SafetyAlerts         *string                  `json:"safety_alerts"`
	Contraindications    *string                  `json:"contraindications"`
	ShippingNotes        *string                  `json:"shipping_notes"`
	Purposes             *[]string                `json:"purposes"`
	ConditionIDs         *[]string                `json:"condition_ids"`
	ShippingOptions      *[]shippingOptionRequest `json:"shipping_options"`
}

func (r advertisementRequest) toInput() seller.AdvertisementInput {
	input := seller.AdvertisementInput{
		CategoryID:           r.CategoryID,
		Title:                r.Title,
		Description:          r.Description,
		Price:                r.Price,
		Stock:                r.Stock,
		PrescriptionRequired: r.PrescriptionRequired,
		CustomMadeAllowed:    r.CustomMadeAllowed,
		Side:                 r.Side,
		Size:                 r.Size,
		Material:             r.Material,
		Rigidity:             r.Rigidity,
		UsageIndication:      r.UsageIndication,
		SafetyAlerts:         r.SafetyAlerts,
		Contraindications:    r.Contraindications,
		ShippingNotes:        r.ShippingNotes,
		PurposeCodes:         r.Purposes,
		ConditionIDs:         r.ConditionIDs,
	}
	if r.ShippingOptions != nil {
		options := make([]seller.ShippingOptionInput, len(*r.ShippingOptions))
		for i, o := range *r.ShippingOptions {
			options[i] = seller.ShippingOptionInput{
				ID:           o.ID,
				Name:         o.Name,
				Carrier:      o.Carrier,
				Price:        o.Price,
				DeliveryDays: o.DeliveryDays,
			}
		}
		input.ShippingOptions = &options
	}
	return input
}

type Service interface {
	seller.ListAdvertisementsUseCase
	seller.GetAdvertisementUseCase
	seller.CreateAdvertisementUseCase
	seller.EditAdvertisementUseCase
	seller.ChangeAdvertisementStatusUseCase
	seller.RemoveAdvertisementUseCase
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

func writeError(c *gin.Context, err error, fallback string) {
	var missing *app.MissingFieldsError
	switch {
	case errors.As(err, &missing):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": app.ErrMissingRequiredFields.Error(), "fields": missing.Fields})
	case errors.Is(err, app.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrAdvertisementNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "advertisement not found"})
	case errors.Is(err, app.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "advertisement belongs to another seller"})
	case errors.Is(err, app.ErrInvalidStatusTransition):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}

func (h *Handler) ListAdvertisements(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	ads, err := h.service.ListAdvertisements(c.Request.Context(), userID, c.Query("status"))
	if err != nil {
		writeError(c, err, "could not get advertisements")
		return
	}

	output := seller.AdvertisementsOutput{Items: make([]*seller.AdvertisementOutput, 0, len(ads))}
	for _, ad := range ads {
		output.Items = append(output.Items, ad.ToOutput())
	}
	c.JSON(http.StatusOK, output)
}

func (h *Handler) GetAdvertisement(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	ad, err := h.service.GetAdvertisement(c.Request.Context(), userID, c.Param("advertisementID"))
	if err != nil {
		writeError(c, err, "could not get advertisement")
		return
	}
	c.JSON(http.StatusOK, ad.ToOutput())
}

func (h *Handler) CreateAdvertisement(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request advertisementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	ad, err := h.service.CreateAdvertisement(c.Request.Context(), userID, request.toInput())
	if err != nil {
		writeError(c, err, "could not create advertisement")
		return
	}
	c.JSON(http.StatusCreated, ad.ToOutput())
}

func (h *Handler) EditAdvertisement(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request advertisementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	ad, err := h.service.EditAdvertisement(c.Request.Context(), userID, c.Param("advertisementID"), request.toInput())
	if err != nil {
		writeError(c, err, "could not edit advertisement")
		return
	}
	c.JSON(http.StatusOK, ad.ToOutput())
}

func (h *Handler) PublishAdvertisement(c *gin.Context) {
	h.changeStatus(c, seller.StatusActionPublish)
}

func (h *Handler) PauseAdvertisement(c *gin.Context) {
	h.changeStatus(c, seller.StatusActionPause)
}

func (h *Handler) ReactivateAdvertisement(c *gin.Context) {
	h.changeStatus(c, seller.StatusActionReactivate)
}

func (h *Handler) changeStatus(c *gin.Context, action seller.StatusAction) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	ad, err := h.service.ChangeAdvertisementStatus(c.Request.Context(), userID, c.Param("advertisementID"), action)
	if err != nil {
		writeError(c, err, "could not change advertisement status")
		return
	}
	c.JSON(http.StatusOK, ad.ToOutput())
}

func (h *Handler) RemoveAdvertisement(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	if err := h.service.RemoveAdvertisement(c.Request.Context(), userID, c.Param("advertisementID")); err != nil {
		writeError(c, err, "could not remove advertisement")
		return
	}
	c.Status(http.StatusNoContent)
}
