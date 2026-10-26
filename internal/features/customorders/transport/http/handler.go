package httpcustomorders

import (
	"context"
	"errors"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/customorders"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

type Service interface {
	customorders.CreateRequestUseCase
	customorders.ListRequestsUseCase
	customorders.GetRequestUseCase
	customorders.BuyerActionsUseCase
	customorders.SellerActionsUseCase
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
	var missingFields *app.MissingFieldsError
	switch {
	case errors.As(err, &missingFields):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": app.ErrMissingRequiredFields.Error(), "fields": missingFields.Fields})
	case errors.Is(err, app.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrConsentRequired):
		c.JSON(http.StatusForbidden, gin.H{"error": "a custom order request requires consent to the custom order terms"})
	case errors.Is(err, app.ErrAdvertisementNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "advertisement not found"})
	case errors.Is(err, app.ErrCustomOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "custom order request not found"})
	case errors.Is(err, app.ErrCustomMadeNotAllowed), errors.Is(err, app.ErrOwnAdvertisement), errors.Is(err, app.ErrInvalidStatusTransition):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}

type listQuery struct {
	Status string `form:"status"`
	Page   uint   `form:"page,default=1" binding:"min=1"`
	Limit  uint   `form:"limit,default=10" binding:"min=1,max=100"`
}

type viewerOutput interface {
	ToOutput(viewerID string) *customorders.RequestDetailOutput
}

func (h *Handler) CreateRequest(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input customorders.RequestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	request, err := h.service.CreateRequest(c.Request.Context(), userID, input)
	if err != nil {
		writeError(c, err, "could not create custom order request")
		return
	}

	c.JSON(http.StatusCreated, request.ToOutput(userID))
}

func (h *Handler) ListRequests(c *gin.Context) {
	h.list(c, h.service.ListRequests)
}

func (h *Handler) ListSellerRequests(c *gin.Context) {
	h.list(c, h.service.ListSellerRequests)
}

func (h *Handler) list(c *gin.Context, list func(context.Context, string, customorders.Status, uint, uint) (*customorders.RequestPage, error)) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var query listQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters: page and limit must be positive integers (limit at most 100)"})
		return
	}

	page, err := list(c.Request.Context(), userID, customorders.Status(query.Status), query.Page, query.Limit)
	if err != nil {
		writeError(c, err, "could not get custom order requests")
		return
	}

	c.JSON(http.StatusOK, page.ToOutput())
}

func (h *Handler) GetRequest(c *gin.Context) {
	h.act(c, "could not get custom order request", func(ctx context.Context, userID string, id string) (viewerOutput, error) {
		return h.service.GetRequest(ctx, userID, id)
	})
}

func (h *Handler) GetSellerRequest(c *gin.Context) {
	h.act(c, "could not get custom order request", func(ctx context.Context, userID string, id string) (viewerOutput, error) {
		return h.service.GetSellerRequest(ctx, userID, id)
	})
}

func (h *Handler) ComplementRequest(c *gin.Context) {
	var input customorders.ComplementInput
	if !bind(c, &input) {
		return
	}
	h.act(c, "could not update custom order request", func(ctx context.Context, userID string, id string) (viewerOutput, error) {
		return h.service.ComplementRequest(ctx, userID, id, input)
	})
}

// CancelRequest takes an optional body with the reason.
func (h *Handler) CancelRequest(c *gin.Context) {
	var input customorders.CancelInput
	if c.Request.ContentLength != 0 && !bind(c, &input) {
		return
	}
	h.act(c, "could not update custom order request", func(ctx context.Context, userID string, id string) (viewerOutput, error) {
		return h.service.CancelRequest(ctx, userID, id, input)
	})
}

func (h *Handler) ReviewRequest(c *gin.Context) {
	h.act(c, "could not update custom order request", func(ctx context.Context, userID string, id string) (viewerOutput, error) {
		return h.service.ReviewRequest(ctx, userID, id)
	})
}

func (h *Handler) AcceptRequest(c *gin.Context) {
	var input customorders.AcceptInput
	if !bind(c, &input) {
		return
	}
	h.act(c, "could not update custom order request", func(ctx context.Context, userID string, id string) (viewerOutput, error) {
		return h.service.AcceptRequest(ctx, userID, id, input)
	})
}

func (h *Handler) RejectRequest(c *gin.Context) {
	var input customorders.ReasonInput
	if !bind(c, &input) {
		return
	}
	h.act(c, "could not update custom order request", func(ctx context.Context, userID string, id string) (viewerOutput, error) {
		return h.service.RejectRequest(ctx, userID, id, input)
	})
}

func (h *Handler) RequestInfo(c *gin.Context) {
	var input customorders.ReasonInput
	if !bind(c, &input) {
		return
	}
	h.act(c, "could not update custom order request", func(ctx context.Context, userID string, id string) (viewerOutput, error) {
		return h.service.RequestInfo(ctx, userID, id, input)
	})
}

func bind(c *gin.Context, input any) bool {
	if err := c.ShouldBindJSON(input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return false
	}
	return true
}

// act runs one call on the request in the path and answers with the request as the user now sees it.
func (h *Handler) act(c *gin.Context, fallback string, call func(ctx context.Context, userID string, id string) (viewerOutput, error)) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	request, err := call(c.Request.Context(), userID, c.Param("requestID"))
	if err != nil {
		writeError(c, err, fallback)
		return
	}

	c.JSON(http.StatusOK, request.ToOutput(userID))
}
