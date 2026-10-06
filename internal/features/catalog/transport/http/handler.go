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
}

type Handler struct {
	getAdvertisements catalog.GetAdvertisementsUseCase
	getAdvertisement  catalog.GetAdvertisementUseCase
	getCategories     catalog.GetCategoriesUseCase
}

func NewHandler(getAdvertisements catalog.GetAdvertisementsUseCase, getAdvertisement catalog.GetAdvertisementUseCase, getCategories catalog.GetCategoriesUseCase) *Handler {
	return &Handler{
		getAdvertisements: getAdvertisements,
		getAdvertisement:  getAdvertisement,
		getCategories:     getCategories,
	}
}

func (h *Handler) GetAdvertisements(c *gin.Context) {
	c.Request.URL.RawQuery = withoutEmptyParams(c.Request.URL.Query()).Encode()

	var query getAdvertisementsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters: page and limit must be positive integers (limit at most 100), prices must be numbers and flags must be booleans"})
		return
	}

	ap, err := h.getAdvertisements.GetAdvertisements(c.Request.Context(), catalog.GetAdvertisementsInput{
		Page:  query.Page,
		Limit: query.Limit,
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
		},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get advertisements"})
		return
	}

	c.JSON(http.StatusOK, ap.ToOutput())
}

func (h *Handler) GetAdvertisement(c *gin.Context) {
	ad, err := h.getAdvertisement.GetAdvertisement(c.Request.Context(), c.Param("advertisementID"))
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
	categories, err := h.getCategories.GetCategories(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get categories"})
		return
	}

	c.JSON(http.StatusOK, categories.ToOutput())
}

func withoutEmptyParams(values url.Values) url.Values {
	for key, vs := range values {
		if len(vs) == 1 && vs[0] == "" {
			delete(values, key)
		}
	}
	return values
}
