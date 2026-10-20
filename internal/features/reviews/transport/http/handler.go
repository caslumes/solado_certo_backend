package httpreviews

import (
	"errors"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/reviews"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

type Service interface {
	reviews.CreateReviewUseCase
	reviews.ListReviewsUseCase
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
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
	case errors.Is(err, app.ErrOrderItemNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "order item not found"})
	case errors.Is(err, app.ErrAdvertisementNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "advertisement not found"})
	case errors.Is(err, app.ErrReviewNotAllowed), errors.Is(err, app.ErrAlreadyReviewed):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}

func (h *Handler) CreateReview(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input reviews.ReviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	review, err := h.service.CreateReview(c.Request.Context(), userID, c.Param("orderID"), c.Param("itemID"), input)
	if err != nil {
		writeError(c, err, "could not create review")
		return
	}
	c.JSON(http.StatusCreated, review.ToOutput())
}

type listReviewsQuery struct {
	Page  uint `form:"page,default=1" binding:"min=1"`
	Limit uint `form:"limit,default=10" binding:"min=1,max=50"`
}

func (h *Handler) ListReviews(c *gin.Context) {
	var query listReviewsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters: page and limit must be positive integers (limit at most 50)"})
		return
	}

	page, err := h.service.ListReviews(c.Request.Context(), c.Param("advertisementID"), query.Page, query.Limit)
	if err != nil {
		writeError(c, err, "could not get reviews")
		return
	}
	c.JSON(http.StatusOK, page.ToOutput())
}
