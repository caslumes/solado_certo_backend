package orders

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

// HistoryFilter narrows an order history (RF-025) to one status and to a range of days in Brasília
// time, both included. The buyer's history goes by order date, the seller's by payment date.
type HistoryFilter struct {
	Status OrderStatus
	From   string
	To     string
}

// OrderQuery is a checked HistoryFilter; Since is included and Until is not.
type OrderQuery struct {
	Status OrderStatus
	Since  *time.Time
	Until  *time.Time
}

func (f HistoryFilter) query(statuses []OrderStatus) (OrderQuery, error) {
	if f.Status != "" && !slices.Contains(statuses, f.Status) {
		names := make([]string, len(statuses))
		for i, status := range statuses {
			names[i] = string(status)
		}
		return OrderQuery{}, fmt.Errorf("%w: status must be one of %s", app.ErrInvalidInput, strings.Join(names, ", "))
	}
	since, err := parseDay("from", f.From)
	if err != nil {
		return OrderQuery{}, err
	}
	until, err := parseDay("to", f.To)
	if err != nil {
		return OrderQuery{}, err
	}
	if until != nil {
		next := until.AddDate(0, 0, 1)
		until = &next
	}
	if since != nil && until != nil && !since.Before(*until) {
		return OrderQuery{}, fmt.Errorf("%w: from must not be after to", app.ErrInvalidInput)
	}
	return OrderQuery{Status: f.Status, Since: since, Until: until}, nil
}

func parseDay(name string, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	day, err := time.ParseInLocation(time.DateOnly, value, brasilia)
	if err != nil {
		return nil, fmt.Errorf("%w: %s must be a date as YYYY-MM-DD", app.ErrInvalidInput, name)
	}
	return &day, nil
}
