package orders

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

const maxCarrierLength = 80

var (
	// correiosPattern is a Correios object code: two letters, nine digits and BR (UC-009).
	correiosPattern = regexp.MustCompile(`^[A-Z]{2}\d{9}BR$`)
	trackingPattern = regexp.MustCompile(`^[A-Z0-9-]{8,40}$`)
)

type ShipmentEntity struct {
	ID           string
	Carrier      string
	TrackingCode string
	Status       string
	PostedAt     *time.Time
	DeliveredAt  *time.Time
}

func (s *ShipmentEntity) ToOutput() *ShipmentOutput {
	if s == nil {
		return nil
	}
	return &ShipmentOutput{
		Carrier:      s.Carrier,
		TrackingCode: s.TrackingCode,
		Status:       s.Status,
		PostedAt:     s.PostedAt,
		DeliveredAt:  s.DeliveredAt,
	}
}

type ShipmentInput struct {
	Carrier      string `json:"carrier"`
	TrackingCode string `json:"tracking_code"`
}

// NewShipment is a shipment recorded together with the order becoming shipped.
type NewShipment struct {
	Carrier      string
	TrackingCode string
}

// ValidateShipment trims the carrier and normalizes the code (upper case, no spaces). A Correios
// code must have the AA123456789BR format; other carriers' codes 8 to 40 letters, digits or hyphens.
func ValidateShipment(input ShipmentInput) (NewShipment, error) {
	carrier := strings.TrimSpace(input.Carrier)
	code := strings.ToUpper(strings.Join(strings.Fields(input.TrackingCode), ""))

	if carrier == "" || len(carrier) > maxCarrierLength {
		return NewShipment{}, fmt.Errorf("%w: carrier must have 1 to %d characters", app.ErrInvalidInput, maxCarrierLength)
	}
	if strings.HasPrefix(strings.ToLower(carrier), "correios") {
		if !correiosPattern.MatchString(code) {
			return NewShipment{}, fmt.Errorf("%w: a Correios tracking code has the format AA123456789BR", app.ErrInvalidTrackingCode)
		}
	} else if !trackingPattern.MatchString(code) {
		return NewShipment{}, fmt.Errorf("%w: a tracking code has 8 to 40 letters, digits or hyphens", app.ErrInvalidTrackingCode)
	}
	return NewShipment{Carrier: carrier, TrackingCode: code}, nil
}
