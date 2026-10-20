package catalog

import "time"

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
	CoverImageURL        *string                     `json:"cover_image_url"`
	AverageRating        *float64                    `json:"average_rating"`
	ReviewCount          uint64                      `json:"review_count"`
	SellerState          *string                     `json:"seller_state"`
}

type SellerOutput struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	State         *string   `json:"state"`
	AverageRating *float64  `json:"average_rating"`
	ReviewCount   uint64    `json:"review_count"`
	MemberSince   time.Time `json:"member_since"`
}

type ShippingOptionOutput struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Carrier      *string `json:"carrier"`
	Price        string  `json:"price"`
	DeliveryDays uint    `json:"delivery_days"`
}

type AdvertisementDetailOutput struct {
	*AdvertisementOutput
	Purposes           []*PurposeOutput        `json:"purposes"`
	Conditions         []*ConditionOutput      `json:"conditions"`
	Seller             *SellerOutput           `json:"seller"`
	ShippingOptions    []*ShippingOptionOutput `json:"shipping_options"`
	SafetyInfoComplete bool                    `json:"safety_info_complete"`
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
	ID                 string            `json:"id"`
	ParentCategoryID   *string           `json:"parent_category_id,omitempty"`
	Children           []*CategoryOutput `json:"children,omitempty"`
	Name               string            `json:"name"`
	Slug               string            `json:"slug"`
	Active             bool              `json:"active"`
	RequiredAttributes []string          `json:"required_attributes"`
}

type CategoriesOutput struct {
	Categories []*CategoryOutput `json:"categories"`
}

type PurposeOutput struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type PurposesOutput struct {
	Purposes []*PurposeOutput `json:"purposes"`
}

type ConditionOutput struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type ConditionsOutput struct {
	Conditions []*ConditionOutput `json:"conditions"`
}
