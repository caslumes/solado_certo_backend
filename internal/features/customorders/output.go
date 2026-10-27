package customorders

import "time"

type RequestSummaryOutput struct {
	ID                 string    `json:"id"`
	Status             string    `json:"status"`
	AdvertisementID    string    `json:"advertisement_id"`
	AdvertisementTitle string    `json:"advertisement_title"`
	CoverURL           *string   `json:"cover_url"`
	BuyerName          string    `json:"buyer_name"`
	SellerName         string    `json:"seller_name"`
	QuotedPrice        *string   `json:"quoted_price"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type HistoryOutput struct {
	PreviousStatus *string   `json:"previous_status"`
	Status         string    `json:"status"`
	ChangedBy      string    `json:"changed_by"`
	Reason         *string   `json:"reason"`
	CreatedAt      time.Time `json:"created_at"`
}

type FittingReportOutput struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

type RequestDetailOutput struct {
	RequestSummaryOutput
	OrderID                *string              `json:"order_id"`
	Measurements           Measurements         `json:"measurements"`
	TechnicalNotes         *string              `json:"technical_notes"`
	RejectionReason        *string              `json:"rejection_reason"`
	AdaptationInstructions *string              `json:"adaptation_instructions"`
	FootPhotoURL           *string              `json:"foot_photo_url"`
	FittingReport          *FittingReportOutput `json:"fitting_report"`
	History                []HistoryOutput      `json:"history"`
	Actions                []string             `json:"actions"`
}

type RequestPageOutput struct {
	Items       []RequestSummaryOutput `json:"items"`
	CurrentPage uint                   `json:"current_page"`
	TotalPages  uint                   `json:"total_pages"`
	TotalItems  uint                   `json:"total_items"`
	HasNext     bool                   `json:"has_next"`
	HasPrevious bool                   `json:"has_previous"`
}
