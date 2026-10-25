package httpdocuments

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/documents"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
	"github.com/gin-gonic/gin"
)

type Service interface {
	documents.UploadPrescriptionUseCase
	documents.OpenDocumentUseCase
	documents.RejectPrescriptionUseCase
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
		c.JSON(http.StatusForbidden, gin.H{"error": "sending a prescription requires consent to the prescription terms"})
	case errors.Is(err, app.ErrOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
	case errors.Is(err, app.ErrOrderItemNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "order item not found"})
	case errors.Is(err, app.ErrDocumentNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
	case errors.Is(err, app.ErrPrescriptionNotNeeded), errors.Is(err, app.ErrInvalidStatusTransition):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, storage.ErrTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("the file must have at most %d MB", app.MaxUploadSize>>20)})
	case errors.Is(err, storage.ErrUnsupportedType):
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "the prescription must be a PDF, JPEG or PNG file"})
	case errors.Is(err, app.ErrUnreadableDocument):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}

func (h *Handler) UploadPrescription(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	file, err := helpers.FormFile(c, "file", app.MaxUploadSize)
	if err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			writeError(c, err, "")
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "send the prescription as multipart form field \"file\""})
		return
	}
	defer file.Close()

	document, err := h.service.UploadPrescription(c.Request.Context(), userID, c.Param("orderID"), c.Param("itemID"), file)
	if err != nil {
		writeError(c, err, "could not save prescription")
		return
	}
	c.JSON(http.StatusCreated, document.ToOutput())
}

// OpenDocument serves the file without caching it anywhere along the way: it is health data.
func (h *Handler) OpenDocument(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	file, err := h.service.OpenDocument(c.Request.Context(), userID, c.Param("documentID"))
	if err != nil {
		writeError(c, err, "could not open document")
		return
	}
	defer file.Close()

	c.DataFromReader(http.StatusOK, file.Size, file.ContentType, file, map[string]string{
		"Cache-Control":          "private, no-store",
		"Content-Disposition":    fmt.Sprintf("inline; filename=%q", file.Name),
		"X-Content-Type-Options": "nosniff",
	})
}

func (h *Handler) RejectPrescription(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input documents.RejectionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	document, err := h.service.RejectPrescription(c.Request.Context(), userID, c.Param("orderID"), c.Param("documentID"), input)
	if err != nil {
		writeError(c, err, "could not reject prescription")
		return
	}
	c.JSON(http.StatusOK, document.ToOutput())
}
