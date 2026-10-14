package orders

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
