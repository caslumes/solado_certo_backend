package httpadmin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/admin"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

type Service interface {
	admin.ListUsersUseCase
	admin.ModerateUserUseCase
	admin.ListAdsUseCase
	admin.RemoveAdUseCase
	admin.ListOrdersUseCase
	admin.ListPaymentsUseCase
	admin.ListAuditLogsUseCase
	admin.GetIndicatorsUseCase
}

type Handler struct {
	service Service
	now     func() time.Time
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service, now: time.Now}
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
	case errors.Is(err, app.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
	case errors.Is(err, app.ErrAdvertisementNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "advertisement not found"})
	case errors.Is(err, app.ErrInvalidStatusTransition):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}

const invalidPaging = "invalid query parameters: page and limit must be positive integers (limit at most 100)"

type paging struct {
	Page  uint `form:"page,default=1" binding:"min=1"`
	Limit uint `form:"limit,default=20" binding:"min=1,max=100"`
}

func (p paging) toPaging() admin.Paging {
	return admin.Paging{Page: p.Page, Limit: p.Limit}
}

type userQuery struct {
	paging
	Search string `form:"q"`
	Status string `form:"status"`
	Role   string `form:"role"`
}

type adQuery struct {
	paging
	Search   string `form:"q"`
	Status   string `form:"status"`
	SellerID string `form:"seller_id"`
}

type orderQuery struct {
	paging
	Search string `form:"q"`
	Status string `form:"status"`
	From   string `form:"from"`
	To     string `form:"to"`
}

type paymentQuery struct {
	paging
	Status        string `form:"status"`
	Method        string `form:"method"`
	RefundGivenUp bool   `form:"refund_given_up"`
	From          string `form:"from"`
	To            string `form:"to"`
}

type auditQuery struct {
	paging
	Entity   string `form:"entity"`
	EntityID string `form:"entity_id"`
	UserID   string `form:"user_id"`
	Action   string `form:"action"`
	From     string `form:"from"`
	To       string `form:"to"`
}

type periodQuery struct {
	From string `form:"from"`
	To   string `form:"to"`
}

type reasonRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) ListUsers(c *gin.Context) {
	var query userQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidPaging})
		return
	}

	page, err := h.service.ListUsers(c.Request.Context(), admin.UserQuery{Search: query.Search, Status: enum.ProfileStatus(query.Status), Role: query.Role, Paging: query.toPaging()})
	if err != nil {
		writeError(c, err, "could not get users")
		return
	}

	now := h.now()
	c.JSON(http.StatusOK, admin.PageToOutput(page, func(u *admin.UserEntity) admin.UserOutput { return u.ToOutput(now) }))
}

func (h *Handler) BlockUser(c *gin.Context) {
	h.moderate(c, h.service.BlockUser, "could not block user")
}

func (h *Handler) SuspendUser(c *gin.Context) {
	h.moderate(c, h.service.SuspendUser, "could not suspend user")
}

func (h *Handler) ReactivateUser(c *gin.Context) {
	h.moderate(c, h.service.ReactivateUser, "could not reactivate user")
}

func (h *Handler) moderate(c *gin.Context, action func(ctx context.Context, adminID string, userID string, input admin.ModerationInput) (*admin.UserEntity, error), fallback string) {
	adminID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input admin.ModerationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "send a JSON body with reason (and until as an RFC 3339 date to suspend)"})
		return
	}

	user, err := action(c.Request.Context(), adminID, c.Param("userID"), input)
	if err != nil {
		writeError(c, err, fallback)
		return
	}

	c.JSON(http.StatusOK, user.ToOutput(h.now()))
}

func (h *Handler) ListAds(c *gin.Context) {
	var query adQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidPaging})
		return
	}

	page, err := h.service.ListAds(c.Request.Context(), admin.AdQuery{Search: query.Search, Status: query.Status, SellerID: query.SellerID, Paging: query.toPaging()})
	if err != nil {
		writeError(c, err, "could not get advertisements")
		return
	}

	c.JSON(http.StatusOK, admin.PageToOutput(page, (*admin.AdEntity).ToOutput))
}

func (h *Handler) RemoveAd(c *gin.Context) {
	adminID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request reasonRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "send a JSON body with reason"})
		return
	}

	ad, err := h.service.RemoveAd(c.Request.Context(), adminID, c.Param("advertisementID"), request.Reason)
	if err != nil {
		writeError(c, err, "could not remove advertisement")
		return
	}

	c.JSON(http.StatusOK, ad.ToOutput())
}

func (h *Handler) ListOrders(c *gin.Context) {
	var query orderQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidPaging})
		return
	}

	page, err := h.service.ListOrders(c.Request.Context(), admin.OrderQuery{
		Search: query.Search, Status: query.Status, From: query.From, To: query.To, Paging: query.toPaging(),
	})
	if err != nil {
		writeError(c, err, "could not get orders")
		return
	}

	c.JSON(http.StatusOK, admin.PageToOutput(page, (*admin.OrderEntity).ToOutput))
}

func (h *Handler) ListPayments(c *gin.Context) {
	var query paymentQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidPaging + "; refund_given_up must be true or false"})
		return
	}

	page, err := h.service.ListPayments(c.Request.Context(), admin.PaymentQuery{
		Status: query.Status, Method: query.Method, RefundGivenUp: query.RefundGivenUp, From: query.From, To: query.To, Paging: query.toPaging(),
	})
	if err != nil {
		writeError(c, err, "could not get payments")
		return
	}

	c.JSON(http.StatusOK, admin.PageToOutput(page, (*admin.PaymentEntity).ToOutput))
}

func (h *Handler) ListAuditLogs(c *gin.Context) {
	var query auditQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": invalidPaging})
		return
	}

	page, err := h.service.ListAuditLogs(c.Request.Context(), admin.AuditQuery{
		Entity: query.Entity, EntityID: query.EntityID, UserID: query.UserID, Action: query.Action,
		From: query.From, To: query.To, Paging: query.toPaging(),
	})
	if err != nil {
		writeError(c, err, "could not get audit logs")
		return
	}

	c.JSON(http.StatusOK, admin.PageToOutput(page, (*admin.AuditEntity).ToOutput))
}

func (h *Handler) GetIndicators(c *gin.Context) {
	var query periodQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters"})
		return
	}

	indicators, err := h.service.GetIndicators(c.Request.Context(), query.From, query.To)
	if err != nil {
		writeError(c, err, "could not get indicators")
		return
	}

	c.JSON(http.StatusOK, indicators.ToOutput())
}
