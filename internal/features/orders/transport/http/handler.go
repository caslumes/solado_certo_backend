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
	orders.PreviewCheckoutUseCase
	orders.CheckoutUseCase
	orders.ListOrdersUseCase
	orders.GetOrderUseCase
	orders.CancelOrderUseCase
	orders.ListSellerOrdersUseCase
	orders.GetSellerOrderUseCase
	orders.StartProcessingUseCase
	orders.ShipOrderUseCase
	orders.ConfirmDeliveryUseCase
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
	writeErrorWith(c, err, fallback, gin.H{})
}

func writeErrorWith(c *gin.Context, err error, fallback string, extra gin.H) {
	var outOfStock *app.OutOfStockError
	var shippingRequired *app.ShippingRequiredError
	var prescriptionRequired *app.PrescriptionRequiredError
	var fittingReportRequired *app.FittingReportRequiredError
	switch {
	case errors.As(err, &outOfStock):
		extra["error"] = app.ErrOutOfStock.Error()
		extra["items"] = outOfStock.Items
		c.JSON(http.StatusConflict, extra)
	case errors.Is(err, app.ErrPricesChanged), errors.Is(err, app.ErrCartChanged):
		extra["error"] = err.Error()
		c.JSON(http.StatusConflict, extra)
	case errors.As(err, &shippingRequired):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": app.ErrShippingRequired.Error(), "advertisement_ids": shippingRequired.AdvertisementIDs})
	case errors.As(err, &prescriptionRequired):
		c.JSON(http.StatusConflict, gin.H{"error": app.ErrPrescriptionRequired.Error(), "item_ids": prescriptionRequired.ItemIDs})
	case errors.As(err, &fittingReportRequired):
		c.JSON(http.StatusConflict, gin.H{"error": app.ErrFittingReportRequired.Error(), "item_ids": fittingReportRequired.ItemIDs})
	case errors.Is(err, app.ErrCartEmpty):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
	case errors.Is(err, app.ErrInvalidStatusTransition):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrAddressNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "address not found"})
	case errors.Is(err, app.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrAdvertisementNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "advertisement not found"})
	case errors.Is(err, app.ErrCartItemNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "item not in cart"})
	case errors.Is(err, app.ErrOwnAdvertisement), errors.Is(err, app.ErrTrackingCodeInUse):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, app.ErrInvalidTrackingCode):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
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

type checkoutRequest struct {
	AddressID string `json:"address_id"`
	Shipping  []struct {
		AdvertisementID  string `json:"advertisement_id"`
		ShippingOptionID string `json:"shipping_option_id"`
	} `json:"shipping"`
}

func (r checkoutRequest) toInput() orders.CheckoutInput {
	input := orders.CheckoutInput{AddressID: r.AddressID, Shipping: make([]orders.ShippingSelection, len(r.Shipping))}
	for i, s := range r.Shipping {
		input.Shipping[i] = orders.ShippingSelection{AdvertisementID: s.AdvertisementID, ShippingOptionID: s.ShippingOptionID}
	}
	return input
}

func (h *Handler) PreviewCheckout(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request checkoutRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	plan, err := h.service.PreviewCheckout(c.Request.Context(), userID, request.toInput())
	if err != nil {
		h.writeCheckoutError(c, userID, err, "could not preview checkout")
		return
	}
	c.JSON(http.StatusOK, plan.ToOutput())
}

func (h *Handler) Checkout(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var request checkoutRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	checkout, created, err := h.service.Checkout(c.Request.Context(), userID, c.GetHeader("Idempotency-Key"), request.toInput())
	if err != nil {
		h.writeCheckoutError(c, userID, err, "could not check out")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, checkout.ToOutput())
}

func (h *Handler) writeCheckoutError(c *gin.Context, userID string, err error, fallback string) {
	extra := gin.H{}
	if errors.Is(err, app.ErrOutOfStock) || errors.Is(err, app.ErrPricesChanged) || errors.Is(err, app.ErrCartChanged) {
		if cart, cartErr := h.service.GetCart(c.Request.Context(), userID); cartErr == nil {
			extra["cart"] = cart.ToOutput()
		}
	}
	writeErrorWith(c, err, fallback, extra)
}

type listOrdersQuery struct {
	Status string `form:"status"`
	From   string `form:"from"`
	To     string `form:"to"`
	Page   uint   `form:"page,default=1" binding:"min=1"`
	Limit  uint   `form:"limit,default=10" binding:"min=1,max=100"`
}

func (q listOrdersQuery) filter() orders.HistoryFilter {
	return orders.HistoryFilter{Status: orders.OrderStatus(q.Status), From: q.From, To: q.To}
}

func (h *Handler) ListOrders(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var query listOrdersQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters: page and limit must be positive integers (limit at most 100)"})
		return
	}

	page, err := h.service.ListOrders(c.Request.Context(), userID, query.filter(), query.Page, query.Limit)
	if err != nil {
		writeError(c, err, "could not get orders")
		return
	}
	c.JSON(http.StatusOK, page.ToOutput())
}

func (h *Handler) GetOrder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	detail, err := h.service.GetOrder(c.Request.Context(), userID, c.Param("orderID"))
	if err != nil {
		writeError(c, err, "could not get order")
		return
	}
	c.JSON(http.StatusOK, detail.ToOutput())
}

func (h *Handler) CancelOrder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	detail, err := h.service.CancelOrder(c.Request.Context(), userID, c.Param("orderID"))
	if err != nil {
		writeError(c, err, "could not cancel order")
		return
	}
	c.JSON(http.StatusOK, detail.ToOutput())
}

func (h *Handler) ConfirmDelivery(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	detail, err := h.service.ConfirmDelivery(c.Request.Context(), userID, c.Param("orderID"))
	if err != nil {
		writeError(c, err, "could not confirm delivery")
		return
	}
	c.JSON(http.StatusOK, detail.ToOutput())
}

func (h *Handler) ListSellerOrders(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var query listOrdersQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters: page and limit must be positive integers (limit at most 100)"})
		return
	}

	page, err := h.service.ListSellerOrders(c.Request.Context(), userID, query.filter(), query.Page, query.Limit)
	if err != nil {
		writeError(c, err, "could not get orders")
		return
	}
	c.JSON(http.StatusOK, page.ToSellerOutput())
}

func (h *Handler) GetSellerOrder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	detail, err := h.service.GetSellerOrder(c.Request.Context(), userID, c.Param("orderID"))
	if err != nil {
		writeError(c, err, "could not get order")
		return
	}
	c.JSON(http.StatusOK, detail.ToSellerOutput())
}

func (h *Handler) StartProcessing(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	detail, err := h.service.StartProcessing(c.Request.Context(), userID, c.Param("orderID"))
	if err != nil {
		writeError(c, err, "could not start processing the order")
		return
	}
	c.JSON(http.StatusOK, detail.ToSellerOutput())
}

func (h *Handler) ShipOrder(c *gin.Context) {
	userID, ok := requireUserID(c)
	if !ok {
		return
	}

	var input orders.ShipmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not decode input"})
		return
	}

	detail, err := h.service.ShipOrder(c.Request.Context(), userID, c.Param("orderID"), input)
	if err != nil {
		writeError(c, err, "could not ship the order")
		return
	}
	c.JSON(http.StatusOK, detail.ToSellerOutput())
}
