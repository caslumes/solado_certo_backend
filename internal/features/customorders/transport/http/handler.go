package httpcustomorders

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/customorders"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
	"github.com/gin-gonic/gin"
)

type Service interface {
	customorders.CreateRequestUseCase
	customorders.ListRequestsUseCase
	customorders.GetRequestUseCase
	customorders.BuyerActionsUseCase
	customorders.SellerActionsUseCase
	customorders.CheckoutUseCase
	customorders.ReportUseCase
	customorders.FootPhotoUseCase
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
	var shippingRequired *app.ShippingRequiredError
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
	case errors.Is(err, app.ErrCustomMadeNotAllowed), errors.Is(err, app.ErrOwnAdvertisement), errors.Is(err, app.ErrInvalidStatusTransition),
		errors.Is(err, app.ErrPricesChanged):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrAddressNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "address not found"})
	case errors.As(err, &shippingRequired):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": app.ErrShippingRequired.Error(), "advertisement_ids": shippingRequired.AdvertisementIDs})
	case errors.Is(err, app.ErrFootPhotoNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "foot photo not found"})
	case errors.Is(err, storage.ErrTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("the file must have at most %d MB", app.MaxUploadSize>>20)})
	case errors.Is(err, app.ErrUnreadableDocument):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
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

func (h *Handler) Checkout(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input customorders.CheckoutInput
	if !bind(c, &input) {
		return
	}

	request, created, err := h.service.Checkout(c.Request.Context(), userID, c.Param("requestID"), c.GetHeader("Idempotency-Key"), input)
	if err != nil {
		writeError(c, err, "could not check out custom order request")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, request.ToOutput(userID))
}

func (h *Handler) ReportRequest(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	file, ok := formFile(c, "report")
	if !ok {
		return
	}
	defer file.Close()

	request, err := h.service.ReportRequest(c.Request.Context(), userID, c.Param("requestID"), c.PostForm("adaptation_instructions"), file)
	if err != nil {
		writeUploadError(c, err, "the report must be a PDF, JPEG or PNG file", "could not save fitting report")
		return
	}
	c.JSON(http.StatusOK, request.ToOutput(userID))
}

func (h *Handler) SendPhoto(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}
	file, ok := formFile(c, "photo")
	if !ok {
		return
	}
	defer file.Close()

	request, err := h.service.SendPhoto(c.Request.Context(), userID, c.Param("requestID"), file)
	if err != nil {
		writeUploadError(c, err, "the photo must be a JPEG or PNG file", "could not save foot photo")
		return
	}
	c.JSON(http.StatusOK, request.ToOutput(userID))
}

func (h *Handler) OpenPhoto(c *gin.Context) {
	h.openPhoto(c, h.service.OpenPhoto)
}

func (h *Handler) OpenSellerPhoto(c *gin.Context) {
	h.openPhoto(c, h.service.OpenSellerPhoto)
}

func (h *Handler) openPhoto(c *gin.Context, open func(context.Context, string, string) (*customorders.PhotoFile, error)) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	file, err := open(c.Request.Context(), userID, c.Param("requestID"))
	if err != nil {
		writeError(c, err, "could not open foot photo")
		return
	}
	defer file.Close()

	c.DataFromReader(http.StatusOK, file.Size, file.ContentType, file, map[string]string{
		"Cache-Control":          "private, no-store",
		"Content-Disposition":    fmt.Sprintf("inline; filename=%q", file.Name),
		"X-Content-Type-Options": "nosniff",
	})
}

func formFile(c *gin.Context, what string) (multipart.File, bool) {
	file, err := helpers.FormFile(c, "file", app.MaxUploadSize)
	if err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			writeError(c, err, "")
			return nil, false
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("send the %s as multipart form field \"file\"", what)})
		return nil, false
	}
	return file, true
}

func writeUploadError(c *gin.Context, err error, unsupported string, fallback string) {
	if errors.Is(err, storage.ErrUnsupportedType) {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": unsupported})
		return
	}
	writeError(c, err, fallback)
}

func bind(c *gin.Context, input any) bool {
	if err := c.ShouldBindJSON(input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return false
	}
	return true
}

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
