package notifications

import "time"

type NotificationOutput struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Data      map[string]string `json:"data"`
	Read      bool              `json:"read"`
	ReadAt    *time.Time        `json:"read_at"`
	CreatedAt time.Time         `json:"created_at"`
}

type NotificationPageOutput struct {
	Items       []NotificationOutput `json:"items"`
	CurrentPage uint                 `json:"current_page"`
	TotalPages  uint                 `json:"total_pages"`
	TotalItems  uint                 `json:"total_items"`
	HasNext     bool                 `json:"has_next"`
	HasPrevious bool                 `json:"has_previous"`
	UnreadCount uint                 `json:"unread_count"`
}
