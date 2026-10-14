package httporders

import (
	"errors"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

type Service interface {
	orders.GetCartUseCase
	orders.AddCartItemUseCase
	orders.ChangeCartItemQuantityUseCase
	orders.RemoveCartItemUseCase
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
	var outOfStock *app.OutOfStockError
	switch {
	case errors.As(err, &outOfStock):
		c.JSON(http.StatusConflict, gin.H{"error": app.ErrOutOfStock.Error(), "items": outOfStock.Items})
	case errors.Is(err, app.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrAdvertisementNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "advertisement not found"})
	case errors.Is(err, app.ErrCartItemNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "item not in cart"})
	case errors.Is(err, app.ErrOwnAdvertisement):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallback})
	}
}

func (h *Handler) GetCart(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	cart, err := h.service.GetCart(c.Request.Context(), userID)
	if err != nil {
		writeError(c, err, "could not get cart")
		return
	}
	c.JSON(http.StatusOK, cart.ToOutput())
}

type addCartItemRequest struct {
	AdvertisementID string `json:"advertisement_id"`
	Quantity        *int   `json:"quantity"`
}

func (h *Handler) AddCartItem(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request addCartItemRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}
	quantity := 1
	if request.Quantity != nil {
		quantity = *request.Quantity
	}

	cart, err := h.service.AddCartItem(c.Request.Context(), userID, request.AdvertisementID, quantity)
	if err != nil {
		writeError(c, err, "could not add item to cart")
		return
	}
	c.JSON(http.StatusOK, cart.ToOutput())
}

type cartItemQuantityRequest struct {
	Quantity *int `json:"quantity"`
}

func (h *Handler) ChangeCartItemQuantity(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request cartItemQuantityRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Quantity == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	cart, err := h.service.ChangeCartItemQuantity(c.Request.Context(), userID, c.Param("advertisementID"), *request.Quantity)
	if err != nil {
		writeError(c, err, "could not change item quantity")
		return
	}
	c.JSON(http.StatusOK, cart.ToOutput())
}

func (h *Handler) RemoveCartItem(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	cart, err := h.service.RemoveCartItem(c.Request.Context(), userID, c.Param("advertisementID"))
	if err != nil {
		writeError(c, err, "could not remove item from cart")
		return
	}
	c.JSON(http.StatusOK, cart.ToOutput())
}
