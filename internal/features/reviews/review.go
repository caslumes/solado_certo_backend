package reviews

import (
	"fmt"
	"strings"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

const (
	minRating     = 1
	maxRating     = 5
	maxTextLength = 2000
	lowComfort    = 2
)

type ReviewInput struct {
	ProductRating      int     `json:"product_rating"`
	SellerRating       int     `json:"seller_rating"`
	ComfortRating      *int    `json:"comfort_rating"`
	Comment            *string `json:"comment"`
	AdaptationFeedback *string `json:"adaptation_feedback"`
}

// ReviewedItem is the order item a review is about. ID is empty when the order has no such item.
type ReviewedItem struct {
	ID          string
	OrderID     string
	BuyerID     string
	OrderStatus string
}

type NewReview struct {
	OrderItemID        string
	ReviewerID         string
	ProductRating      int
	SellerRating       int
	ComfortRating      *int
	Comment            *string
	AdaptationFeedback *string
}

// newReview checks the ratings (1 to 5, comfort optional) and trims the texts; an empty text is no text.
func newReview(itemID string, reviewerID string, input ReviewInput) (NewReview, error) {
	var invalid []string
	if input.ProductRating < minRating || input.ProductRating > maxRating {
		invalid = append(invalid, "product_rating")
	}
	if input.SellerRating < minRating || input.SellerRating > maxRating {
		invalid = append(invalid, "seller_rating")
	}
	if input.ComfortRating != nil && (*input.ComfortRating < minRating || *input.ComfortRating > maxRating) {
		invalid = append(invalid, "comfort_rating")
	}
	if len(invalid) > 0 {
		return NewReview{}, fmt.Errorf("%w: %s must be between %d and %d", app.ErrInvalidInput, strings.Join(invalid, ", "), minRating, maxRating)
	}

	comment, err := text("comment", input.Comment)
	if err != nil {
		return NewReview{}, err
	}
	feedback, err := text("adaptation_feedback", input.AdaptationFeedback)
	if err != nil {
		return NewReview{}, err
	}
	return NewReview{
		OrderItemID:        itemID,
		ReviewerID:         reviewerID,
		ProductRating:      input.ProductRating,
		SellerRating:       input.SellerRating,
		ComfortRating:      input.ComfortRating,
		Comment:            comment,
		AdaptationFeedback: feedback,
	}, nil
}

func text(field string, value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if len([]rune(trimmed)) > maxTextLength {
		return nil, fmt.Errorf("%w: %s must have at most %d characters", app.ErrInvalidInput, field, maxTextLength)
	}
	return &trimmed, nil
}

type ReviewEntity struct {
	ID                 string
	OrderItemID        string
	ProductRating      int
	SellerRating       int
	ComfortRating      *int
	Comment            *string
	AdaptationFeedback *string
	CreatedAt          time.Time
}

func (r *ReviewEntity) ToOutput() ReviewOutput {
	return ReviewOutput{
		ID:                 r.ID,
		OrderItemID:        r.OrderItemID,
		ProductRating:      r.ProductRating,
		SellerRating:       r.SellerRating,
		ComfortRating:      r.ComfortRating,
		Comment:            r.Comment,
		AdaptationFeedback: r.AdaptationFeedback,
		SuggestSupport:     r.ComfortRating != nil && *r.ComfortRating <= lowComfort,
		CreatedAt:          r.CreatedAt,
	}
}

// PublicReviewEntity is a review as anyone sees it on the ad: the reviewer's first name only, and
// no adaptation feedback, which may describe the buyer's health.
type PublicReviewEntity struct {
	ID            string
	ReviewerName  string
	ProductRating int
	SellerRating  int
	ComfortRating *int
	Comment       *string
	CreatedAt     time.Time
}

func (r *PublicReviewEntity) ToOutput() PublicReviewOutput {
	return PublicReviewOutput{
		ID:            r.ID,
		ReviewerName:  r.ReviewerName,
		ProductRating: r.ProductRating,
		SellerRating:  r.SellerRating,
		ComfortRating: r.ComfortRating,
		Comment:       r.Comment,
		CreatedAt:     r.CreatedAt,
	}
}

type ReviewPage struct {
	Items       []*PublicReviewEntity
	CurrentPage uint
	TotalPages  uint
	TotalItems  uint
}

func (p *ReviewPage) ToOutput() *ReviewPageOutput {
	items := make([]PublicReviewOutput, len(p.Items))
	for i, review := range p.Items {
		items[i] = review.ToOutput()
	}
	return &ReviewPageOutput{
		Items:       items,
		CurrentPage: p.CurrentPage,
		TotalPages:  p.TotalPages,
		TotalItems:  p.TotalItems,
		HasNext:     p.CurrentPage < p.TotalPages,
		HasPrevious: p.CurrentPage > 1,
	}
}
