package orders

import "time"

type ShippingOptionOutput struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Carrier      *string `json:"carrier"`
	Price        string  `json:"price"`
	DeliveryDays int     `json:"delivery_days"`
}

type SellerOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CartItemOutput struct {
	AdvertisementID string                 `json:"advertisement_id"`
	Title           string                 `json:"title"`
	CoverImageURL   *string                `json:"cover_image_url"`
	Quantity        int                    `json:"quantity"`
	UnitPrice       string                 `json:"unit_price"`
	AddedUnitPrice  string                 `json:"added_unit_price"`
	PriceChanged    bool                   `json:"price_changed"`
	Stock           int                    `json:"stock"`
	Available       bool                   `json:"available"`
	Problem         *ItemProblem           `json:"problem"`
	Subtotal        string                 `json:"subtotal"`
	ShippingOptions []ShippingOptionOutput `json:"shipping_options"`
}

type CartGroupOutput struct {
	Seller   SellerOutput     `json:"seller"`
	Items    []CartItemOutput `json:"items"`
	Subtotal string           `json:"subtotal"`
}

type CartOutput struct {
	ID              *string           `json:"id"`
	Groups          []CartGroupOutput `json:"groups"`
	ItemCount       int               `json:"item_count"`
	Subtotal        string            `json:"subtotal"`
	HasProblems     bool              `json:"has_problems"`
	HasPriceChanges bool              `json:"has_price_changes"`
}

type ItemReviewOutput struct {
	ProductRating      int       `json:"product_rating"`
	SellerRating       int       `json:"seller_rating"`
	ComfortRating      *int      `json:"comfort_rating"`
	Comment            *string   `json:"comment"`
	AdaptationFeedback *string   `json:"adaptation_feedback"`
	CreatedAt          time.Time `json:"created_at"`
}

type OrderItemOutput struct {
	ID              string               `json:"id,omitempty"`
	AdvertisementID string               `json:"advertisement_id"`
	Title           string               `json:"title"`
	CoverImageURL   *string              `json:"cover_image_url"`
	Quantity        int                  `json:"quantity"`
	UnitPrice       string               `json:"unit_price"`
	Subtotal        string               `json:"subtotal"`
	ShippingOption  ShippingOptionOutput `json:"shipping_option"`
	Review          *ItemReviewOutput    `json:"review"`
}

type PlannedOrderOutput struct {
	Seller        SellerOutput      `json:"seller"`
	Items         []OrderItemOutput `json:"items"`
	Subtotal      string            `json:"subtotal"`
	ShippingTotal string            `json:"shipping_total"`
	DiscountTotal string            `json:"discount_total"`
	Total         string            `json:"total"`
}

type CheckoutPreviewOutput struct {
	Address       AddressSnapshot      `json:"address"`
	Orders        []PlannedOrderOutput `json:"orders"`
	Subtotal      string               `json:"subtotal"`
	ShippingTotal string               `json:"shipping_total"`
	DiscountTotal string               `json:"discount_total"`
	Total         string               `json:"total"`
}

type OrderOutput struct {
	ID            string            `json:"id"`
	Number        string            `json:"number"`
	Status        string            `json:"status"`
	Seller        SellerOutput      `json:"seller"`
	Address       AddressSnapshot   `json:"address"`
	Items         []OrderItemOutput `json:"items"`
	Subtotal      string            `json:"subtotal"`
	ShippingTotal string            `json:"shipping_total"`
	DiscountTotal string            `json:"discount_total"`
	Total         string            `json:"total"`
	PaidAt        *time.Time        `json:"paid_at"`
	DeliveredAt   *time.Time        `json:"delivered_at"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type CheckoutOutput struct {
	ID     string        `json:"id"`
	Orders []OrderOutput `json:"orders"`
	Total  string        `json:"total"`
}

type StatusChangeOutput struct {
	From      *string   `json:"from"`
	To        string    `json:"to"`
	Reason    *string   `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type ShipmentOutput struct {
	Carrier      string     `json:"carrier"`
	TrackingCode string     `json:"tracking_code"`
	Status       string     `json:"status"`
	PostedAt     *time.Time `json:"posted_at"`
	DeliveredAt  *time.Time `json:"delivered_at"`
}

type OrderDetailOutput struct {
	OrderOutput
	History            []StatusChangeOutput `json:"history"`
	Shipment           *ShipmentOutput      `json:"shipment"`
	CanCancel          bool                 `json:"can_cancel"`
	CanConfirmDelivery bool                 `json:"can_confirm_delivery"`
}

type BuyerOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SellerOrderOutput is an order as its seller sees it, with the buyer.
type SellerOrderOutput struct {
	OrderOutput
	Buyer BuyerOutput `json:"buyer"`
}

type SellerOrderDetailOutput struct {
	SellerOrderOutput
	History            []StatusChangeOutput `json:"history"`
	Shipment           *ShipmentOutput      `json:"shipment"`
	CanStartProcessing bool                 `json:"can_start_processing"`
	CanShip            bool                 `json:"can_ship"`
}

type OrderPageOutput struct {
	Items       []OrderOutput `json:"items"`
	CurrentPage uint          `json:"current_page"`
	TotalPages  uint          `json:"total_pages"`
	TotalItems  uint          `json:"total_items"`
	HasNext     bool          `json:"has_next"`
	HasPrevious bool          `json:"has_previous"`
}

type SellerOrderPageOutput struct {
	Items       []SellerOrderOutput `json:"items"`
	CurrentPage uint                `json:"current_page"`
	TotalPages  uint                `json:"total_pages"`
	TotalItems  uint                `json:"total_items"`
	HasNext     bool                `json:"has_next"`
	HasPrevious bool                `json:"has_previous"`
}
