package httpcatalog

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	"github.com/gin-gonic/gin"
)

type getAdvertisementsQuery struct {
	Page                 uint     `form:"page,default=1" binding:"min=1"`
	Limit                uint     `form:"limit,default=10" binding:"min=1,max=100"`
	Sort                 string   `form:"sort" binding:"omitempty,oneof=relevance price_asc price_desc newest"`
	Query                *string  `form:"q"`
	Category             *string  `form:"category"`
	PriceMin             *float64 `form:"price_min"`
	PriceMax             *float64 `form:"price_max"`
	PrescriptionRequired *bool    `form:"prescription_required"`
	CustomMadeAllowed    *bool    `form:"custom_made_allowed"`
	Side                 *string  `form:"side"`
	Size                 *string  `form:"size"`
	Material             *string  `form:"material"`
	Rigidity             *string  `form:"rigidity"`
	UsageIndication      *string  `form:"usage_indication"`
	Purposes             []string `form:"purpose"`
	Conditions           []string `form:"condition"`
}

type Service interface {
	catalog.GetAdvertisementsUseCase
	catalog.GetAdvertisementUseCase
	catalog.GetCategoriesUseCase
	catalog.GetPurposesUseCase
	catalog.GetConditionsUseCase
}

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetAdvertisements(c *gin.Context) {
	c.Request.URL.RawQuery = withoutEmptyParams(c.Request.URL.Query()).Encode()

	var query getAdvertisementsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters: page and limit must be positive integers (limit at most 100), prices must be numbers and flags must be booleans and sort must be relevance, price_asc, price_desc or newest"})
		return
	}

	ap, err := h.service.GetAdvertisements(c.Request.Context(), catalog.GetAdvertisementsInput{
		Page:  query.Page,
		Limit: query.Limit,
		Sort:  catalog.AdvertisementSort(query.Sort),
		Filters: &catalog.AdvertisementFilters{
			Query:                query.Query,
			Category:             query.Category,
			PriceMin:             query.PriceMin,
			PriceMax:             query.PriceMax,
			PrescriptionRequired: query.PrescriptionRequired,
			CustomMadeAllowed:    query.CustomMadeAllowed,
			Side:                 query.Side,
			Size:                 query.Size,
			Material:             query.Material,
			Rigidity:             query.Rigidity,
			UsageIndication:      query.UsageIndication,
			Purposes:             withoutEmpty(query.Purposes),
			Conditions:           withoutEmpty(query.Conditions),
		},
	})
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get advertisements"})
		return
	}

	c.JSON(http.StatusOK, ap.ToOutput())
}

func (h *Handler) GetAdvertisement(c *gin.Context) {
	ad, err := h.service.GetAdvertisement(c.Request.Context(), c.Param("advertisementID"))
	if err != nil {
		if errors.Is(err, app.ErrAdvertisementNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "advertisement not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get advertisement"})
		return
	}

	c.JSON(http.StatusOK, ad.ToOutput())
}

func (h *Handler) GetCategories(c *gin.Context) {
	categories, err := h.service.GetCategories(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get categories"})
		return
	}

	c.JSON(http.StatusOK, categories.ToOutput())
}

func (h *Handler) GetPurposes(c *gin.Context) {
	purposes, err := h.service.GetPurposes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get purposes"})
		return
	}

	output := catalog.PurposesOutput{Purposes: make([]*catalog.PurposeOutput, 0, len(purposes))}
	for _, purpose := range purposes {
		output.Purposes = append(output.Purposes, purpose.ToOutput())
	}
	c.JSON(http.StatusOK, output)
}

func (h *Handler) GetConditions(c *gin.Context) {
	conditions, err := h.service.GetConditions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get conditions"})
		return
	}

	output := catalog.ConditionsOutput{Conditions: make([]*catalog.ConditionOutput, 0, len(conditions))}
	for _, condition := range conditions {
		output.Conditions = append(output.Conditions, condition.ToOutput())
	}
	c.JSON(http.StatusOK, output)
}

func withoutEmpty(values []string) []string {
	var result []string
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func withoutEmptyParams(values url.Values) url.Values {
	for key, vs := range values {
		if len(vs) == 1 && vs[0] == "" {
			delete(values, key)
		}
	}
	return values
}
