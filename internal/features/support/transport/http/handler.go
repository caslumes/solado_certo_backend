package httpsupport

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/support"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
	"github.com/gin-gonic/gin"
)

type Service interface {
	support.CreateTicketUseCase
	support.ListTicketsUseCase
	support.GetTicketUseCase
	support.AddMessageUseCase
	support.OpenAttachmentUseCase
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
	case errors.Is(err, app.ErrOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
	case errors.Is(err, app.ErrTicketNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "support ticket not found"})
	case errors.Is(err, app.ErrAttachmentNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "attachment not found"})
	case errors.Is(err, app.ErrInvalidStatusTransition):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, storage.ErrTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("the file must have at most %d MB", app.MaxUploadSize>>20)})
	case errors.Is(err, storage.ErrUnsupportedType):
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "the attachment must be a PDF, JPEG or PNG file"})
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

type messageRequest struct {
	Message string `json:"message"`
}

func (h *Handler) CreateTicket(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input support.TicketInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	ticket, err := h.service.CreateTicket(c.Request.Context(), userID, input)
	if err != nil {
		writeError(c, err, "could not open support ticket")
		return
	}

	c.JSON(http.StatusCreated, ticket.ToOutput(userID))
}

func (h *Handler) ListTickets(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var query listQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters: page and limit must be positive integers (limit at most 100)"})
		return
	}

	page, err := h.service.ListTickets(c.Request.Context(), userID, support.Status(query.Status), query.Page, query.Limit)
	if err != nil {
		writeError(c, err, "could not get support tickets")
		return
	}

	c.JSON(http.StatusOK, page.ToOutput(userID))
}

func (h *Handler) GetTicket(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	ticket, err := h.service.GetTicket(c.Request.Context(), userID, c.Param("ticketID"))
	if err != nil {
		writeError(c, err, "could not get support ticket")
		return
	}

	c.JSON(http.StatusOK, ticket.ToOutput(userID))
}

func (h *Handler) AddMessage(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var message string
	var attachment io.Reader
	if strings.HasPrefix(c.ContentType(), "multipart/") {
		file, err := helpers.FormFile(c, "file", app.MaxUploadSize)
		switch {
		case errors.Is(err, storage.ErrTooLarge):
			writeError(c, err, "")
			return
		case err == nil:
			defer file.Close()
			attachment = file
		}
		message = c.PostForm("message")
	} else {
		var request messageRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "send the message as JSON or as multipart form fields \"message\" and \"file\""})
			return
		}
		message = request.Message
	}

	ticket, err := h.service.AddMessage(c.Request.Context(), userID, c.Param("ticketID"), message, attachment)
	if err != nil {
		writeError(c, err, "could not send message")
		return
	}

	c.JSON(http.StatusCreated, ticket.ToOutput(userID))
}

func (h *Handler) OpenAttachment(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	file, err := h.service.OpenAttachment(c.Request.Context(), userID, c.Param("ticketID"), c.Param("messageID"))
	if err != nil {
		writeError(c, err, "could not open attachment")
		return
	}
	defer file.Close()

	c.DataFromReader(http.StatusOK, file.Size, file.ContentType, file, map[string]string{
		"Cache-Control":          "private, no-store",
		"Content-Disposition":    fmt.Sprintf("inline; filename=%q", file.Name),
		"X-Content-Type-Options": "nosniff",
	})
}
