package catalog

type AdvertisementImageOutput struct {
	URL     string `json:"url"`
	AltText string `json:"alt_text"`
	Order   int    `json:"order"`
	IsCover bool   `json:"is_cover"`
}

type AdvertisementOutput struct {
	ID                   string                      `json:"id"`
	SellerID             string                      `json:"seller_id"`
	CategoryID           string                      `json:"category_id"`
	Title                string                      `json:"title"`
	Description          string                      `json:"description"`
	Price                string                      `json:"price"`
	Stock                uint64                      `json:"stock"`
	Status               string                      `json:"status"`
	PrescriptionRequired bool                        `json:"prescription_required"`
	CustomMadeAllowed    bool                        `json:"custom_made_allowed"`
	Images               []*AdvertisementImageOutput `json:"images"`
	Side                 *string                     `json:"side"`
	Size                 *string                     `json:"size"`
	Material             *string                     `json:"material"`
	Rigidity             *string                     `json:"rigidity"`
	UsageIndication      *string                     `json:"usage_indication"`
	SafetyAlerts         *string                     `json:"safety_alerts"`
	Contraindications    *string                     `json:"contraindications"`
	ShippingNotes        *string                     `json:"shipping_notes"`
}

type AdvertisementPageOutput struct {
	Items       []*AdvertisementOutput `json:"items"`
	CurrentPage uint                   `json:"current_page"`
	TotalPages  uint                   `json:"total_pages"`
	TotalItems  uint                   `json:"total_items"`
	HasNext     bool                   `json:"has_next"`
	HasPrevious bool                   `json:"has_previous"`
}

type CategoryOutput struct {
	ID               string            `json:"id"`
	ParentCategoryID *string           `json:"parent_category_id,omitempty"`
	Children         []*CategoryOutput `json:"children,omitempty"`
	Name             string            `json:"name"`
	Slug             string            `json:"slug"`
	Active           bool              `json:"active"`
}

type CategoriesOutput struct {
	Categories []*CategoryOutput `json:"categories"`
}
