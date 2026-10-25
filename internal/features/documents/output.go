package documents

import "time"

type DocumentOutput struct {
	ID              string    `json:"id"`
	OrderID         string    `json:"order_id"`
	OrderItemID     *string   `json:"order_item_id"`
	Type            string    `json:"type"`
	Status          string    `json:"status"`
	RejectionReason *string   `json:"rejection_reason"`
	URL             string    `json:"url"`
	CreatedAt       time.Time `json:"created_at"`
}
