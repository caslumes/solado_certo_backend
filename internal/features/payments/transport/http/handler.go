package httppayments

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/payments"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

type Service interface {
	payments.CreatePaymentUseCase
	payments.ListPaymentsUseCase
	payments.ApplyPaymentStatusUseCase
}

// webhookTimeout bounds the work a webhook starts: reading the charge back, with the client's retries.
const webhookTimeout = time.Minute

type Handler struct {
	service      Service
	webhookToken string
	webhooks     sync.WaitGroup
}

// NewHandler takes the token Asaas sends in the asaas-access-token header; webhooks are refused
// while it is empty.
func NewHandler(service Service, webhookToken string) *Handler {
	return &Handler{service: service, webhookToken: webhookToken}
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
	case errors.Is(err, app.ErrInvalidStatusTransition), errors.Is(err, app.ErrOrderExpired),
		errors.Is(err, app.ErrPaymentInProgress), errors.Is(err, app.ErrTaxIDInUse):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrTaxIDRequired), errors.Is(err, app.ErrIdempotencyKeyReused):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrGatewayUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}

// CreatePayment answers 201 with the new attempt, or 200 with the same attempt when the
// Idempotency-Key was already used. A refused card is a 201 with the attempt failed.
func (h *Handler) CreatePayment(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input payments.CreatePaymentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}
	input.RemoteIP = c.ClientIP()

	payment, created, err := h.service.CreatePayment(c.Request.Context(), userID, c.Param("orderID"), c.GetHeader("Idempotency-Key"), input)
	if err != nil {
		writeError(c, err, "could not create payment")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, payment.ToOutput())
}

func (h *Handler) ListPayments(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	list, err := h.service.ListPayments(c.Request.Context(), userID, c.Param("orderID"))
	if err != nil {
		writeError(c, err, "could not get payments")
		return
	}
	c.JSON(http.StatusOK, payments.PaymentsToOutput(list))
}

type asaasEvent struct {
	Event   string `json:"event"`
	Payment struct {
		ID string `json:"id"`
	} `json:"payment"`
}

// AsaasWebhook answers at once and applies the status in the background: Asaas stops delivering
// after repeated slow or failed answers, and the poller covers an event whose work fails. Only the
// charge's id is used; its status is read back from Asaas.
func (h *Handler) AsaasWebhook(c *gin.Context) {
	token := c.GetHeader("asaas-access-token")
	if h.webhookToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(h.webhookToken)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook token"})
		return
	}

	var event asaasEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}
	if !strings.HasPrefix(event.Event, "PAYMENT_") || event.Payment.ID == "" {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	ctx := context.WithoutCancel(c.Request.Context())
	h.webhooks.Go(func() {
		ctx, cancel := context.WithTimeout(ctx, webhookTimeout)
		defer cancel()
		err := h.service.ApplyPaymentStatus(ctx, event.Payment.ID, payments.SourceWebhook)
		if errors.Is(err, app.ErrPaymentNotFound) {
			slog.InfoContext(ctx, "webhook for an unknown charge", "event", event.Event, "transaction_id", event.Payment.ID)
		}
	})
	c.JSON(http.StatusOK, gin.H{"received": true})
}

// Wait blocks until the webhooks being applied are done, so shutting down doesn't cut them off.
func (h *Handler) Wait() {
	h.webhooks.Wait()
}
