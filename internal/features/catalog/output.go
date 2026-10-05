package catalog

type AdvertisementImageOutput struct {
	URL     string `json:"url"`
	AltText string `json:"alt_text"`
	Order   int    `json:"order"`
	IsCover bool   `json:"is_cover"`
}

type AdvertisementOutput struct {
	SellerID             string                      `json:"seller_id"`
	CategoryID           string                      `json:"category_id"`
	Title                string                      `json:"title"`
	Description          string                      `json:"description"`
	Price                float64                     `json:"price"`
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
	Items      []*AdvertisementOutput `json:"items"`
	NextCursor *string                `json:"next_cursor,omitempty"`
	HasNext    bool                   `json:"has_next"`
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
