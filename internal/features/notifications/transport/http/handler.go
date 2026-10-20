package httpnotifications

import (
	"errors"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

type Service interface {
	notifications.ListNotificationsUseCase
	notifications.MarkReadUseCase
	notifications.MarkAllReadUseCase
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
	case errors.Is(err, app.ErrNotificationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}

type listNotificationsQuery struct {
	Page  uint `form:"page,default=1" binding:"min=1"`
	Limit uint `form:"limit,default=20" binding:"min=1,max=100"`
}

func (h *Handler) ListNotifications(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var query listNotificationsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters: page and limit must be positive integers (limit at most 100)"})
		return
	}

	page, err := h.service.ListNotifications(c.Request.Context(), userID, query.Page, query.Limit)
	if err != nil {
		writeError(c, err, "could not get notifications")
		return
	}
	c.JSON(http.StatusOK, page.ToOutput())
}

func (h *Handler) MarkRead(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	if err := h.service.MarkRead(c.Request.Context(), userID, c.Param("notificationID")); err != nil {
		writeError(c, err, "could not mark notification as read")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) MarkAllRead(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	if err := h.service.MarkAllRead(c.Request.Context(), userID); err != nil {
		writeError(c, err, "could not mark notifications as read")
		return
	}
	c.Status(http.StatusNoContent)
}
