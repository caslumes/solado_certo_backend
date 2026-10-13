package seller

import "time"

type ImageOutput struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	AltText  string `json:"alt_text"`
	Position int    `json:"position"`
	IsCover  bool   `json:"is_cover"`
}

type ShippingOptionOutput struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Carrier      *string `json:"carrier"`
	Price        string  `json:"price"`
	DeliveryDays int     `json:"delivery_days"`
}

type AdvertisementOutput struct {
	ID                   string                 `json:"id"`
	CategoryID           string                 `json:"category_id"`
	Title                string                 `json:"title"`
	Description          string                 `json:"description"`
	Price                string                 `json:"price"`
	Stock                int                    `json:"stock"`
	Status               string                 `json:"status"`
	SoldOut              bool                   `json:"sold_out"`
	PrescriptionRequired bool                   `json:"prescription_required"`
	CustomMadeAllowed    bool                   `json:"custom_made_allowed"`
	Side                 *string                `json:"side"`
	Size                 *string                `json:"size"`
	Material             *string                `json:"material"`
	Rigidity             *string                `json:"rigidity"`
	UsageIndication      *string                `json:"usage_indication"`
	SafetyAlerts         *string                `json:"safety_alerts"`
	Contraindications    *string                `json:"contraindications"`
	ShippingNotes        *string                `json:"shipping_notes"`
	Purposes             []string               `json:"purposes"`
	ConditionIDs         []string               `json:"condition_ids"`
	ShippingOptions      []ShippingOptionOutput `json:"shipping_options"`
	Images               []ImageOutput          `json:"images"`
	CoverImageURL        *string                `json:"cover_image_url"`
	MissingFields        []string               `json:"missing_fields"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
}

type AdvertisementsOutput struct {
	Items []*AdvertisementOutput `json:"items"`
}
