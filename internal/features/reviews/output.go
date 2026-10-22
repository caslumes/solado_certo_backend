package reviews

import "time"

type ReviewOutput struct {
	ID                 string    `json:"id"`
	OrderItemID        string    `json:"order_item_id"`
	ProductRating      int       `json:"product_rating"`
	SellerRating       int       `json:"seller_rating"`
	ComfortRating      *int      `json:"comfort_rating"`
	Comment            *string   `json:"comment"`
	AdaptationFeedback *string   `json:"adaptation_feedback"`
	SuggestSupport     bool      `json:"suggest_support"`
	CreatedAt          time.Time `json:"created_at"`
}

type PublicReviewOutput struct {
	ID            string    `json:"id"`
	ReviewerName  string    `json:"reviewer_name"`
	ProductRating int       `json:"product_rating"`
	SellerRating  int       `json:"seller_rating"`
	ComfortRating *int      `json:"comfort_rating"`
	Comment       *string   `json:"comment"`
	CreatedAt     time.Time `json:"created_at"`
}

type ReviewPageOutput struct {
	Items       []PublicReviewOutput `json:"items"`
	CurrentPage uint                 `json:"current_page"`
	TotalPages  uint                 `json:"total_pages"`
	TotalItems  uint                 `json:"total_items"`
	HasNext     bool                 `json:"has_next"`
	HasPrevious bool                 `json:"has_previous"`
}
