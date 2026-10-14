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

type OrderItemOutput struct {
	AdvertisementID string               `json:"advertisement_id"`
	Title           string               `json:"title"`
	CoverImageURL   *string              `json:"cover_image_url"`
	Quantity        int                  `json:"quantity"`
	UnitPrice       string               `json:"unit_price"`
	Subtotal        string               `json:"subtotal"`
	ShippingOption  ShippingOptionOutput `json:"shipping_option"`
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
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type CheckoutOutput struct {
	ID     string        `json:"id"`
	Orders []OrderOutput `json:"orders"`
	Total  string        `json:"total"`
}
