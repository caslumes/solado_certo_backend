package customorders

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

type Status string

const (
	StatusRequested    Status = "requested"
	StatusUnderReview  Status = "under_review"
	StatusNeedsInfo    Status = "needs_info"
	StatusApproved     Status = "approved"
	StatusRejected     Status = "rejected"
	StatusInProduction Status = "in_production"
	StatusCompleted    Status = "completed"
	StatusCancelled    Status = "cancelled"
)

var statuses = []Status{
	StatusRequested, StatusUnderReview, StatusNeedsInfo, StatusApproved, StatusRejected, StatusInProduction, StatusCompleted, StatusCancelled,
}

// Action is something the buyer or the seller does to a request. Each one moves it to one status.
type Action string

const (
	ActionReview      Action = "review"
	ActionAccept      Action = "accept"
	ActionReject      Action = "reject"
	ActionRequestInfo Action = "request_info"
	ActionComplement  Action = "complement"
	ActionCancel      Action = "cancel"
)

type transition struct {
	from []Status
	to   Status
}

// transitions is the custom order status machine (UC-008). The seller reviews, accepts, rejects or asks
// for more information; the buyer complements what was asked or cancels before paying. Checkout and the
// fitting report (task 3) take an approved request to in_production and completed.
var transitions = map[Action]transition{
	ActionReview:      {from: []Status{StatusRequested}, to: StatusUnderReview},
	ActionAccept:      {from: []Status{StatusRequested, StatusUnderReview}, to: StatusApproved},
	ActionReject:      {from: []Status{StatusRequested, StatusUnderReview, StatusNeedsInfo}, to: StatusRejected},
	ActionRequestInfo: {from: []Status{StatusRequested, StatusUnderReview}, to: StatusNeedsInfo},
	ActionComplement:  {from: []Status{StatusNeedsInfo}, to: StatusUnderReview},
	ActionCancel:      {from: []Status{StatusRequested, StatusUnderReview, StatusNeedsInfo, StatusApproved}, to: StatusCancelled},
}

var (
	sellerActions = []Action{ActionReview, ActionAccept, ActionReject, ActionRequestInfo}
	buyerActions  = []Action{ActionComplement, ActionCancel}
)

func (s Status) Allows(action Action) bool {
	return slices.Contains(transitions[action].from, s)
}

type ArchType string

const (
	ArchTypeFlat    ArchType = "flat"
	ArchTypeNormal  ArchType = "normal"
	ArchTypeHigh    ArchType = "high"
	ArchTypeUnknown ArchType = "unknown"
)

const (
	minFootLength = 10.0
	maxFootLength = 35.0
	minFootWidth  = 4.0
	maxFootWidth  = 15.0
	minShoeSize   = 15
	maxShoeSize   = 50

	maxNotesLength   = 2000
	maxMessageLength = 1000
	// maxQuotedPrice is the largest value of numeric(10,2).
	maxQuotedPrice helpers.Cents = 99_999_999_99
)

type Foot struct {
	LengthCm float64 `json:"length_cm"`
	WidthCm  float64 `json:"width_cm"`
}

// Measurements are what the seller needs to make the item: each foot's length and width, the arch
// type and the usual shoe size (BR).
type Measurements struct {
	Left     *Foot    `json:"left"`
	Right    *Foot    `json:"right"`
	ArchType ArchType `json:"arch_type"`
	ShoeSize int      `json:"shoe_size"`
}

// check validates the measurements and rounds the lengths to millimetres.
func (m Measurements) check() (Measurements, error) {
	var missing []string
	if m.Left == nil {
		missing = append(missing, "measurements.left")
	}
	if m.Right == nil {
		missing = append(missing, "measurements.right")
	}
	if m.ArchType == "" {
		missing = append(missing, "measurements.arch_type")
	}
	if m.ShoeSize == 0 {
		missing = append(missing, "measurements.shoe_size")
	}
	if len(missing) > 0 {
		return Measurements{}, &app.MissingFieldsError{Fields: missing}
	}

	left, invalid := checkFoot("left", *m.Left)
	right, invalidRight := checkFoot("right", *m.Right)
	invalid = append(invalid, invalidRight...)

	switch m.ArchType {
	case ArchTypeFlat, ArchTypeNormal, ArchTypeHigh, ArchTypeUnknown:
	default:
		invalid = append(invalid, "measurements.arch_type must be one of flat, normal, high, unknown")
	}

	if m.ShoeSize < minShoeSize || m.ShoeSize > maxShoeSize {
		invalid = append(invalid, fmt.Sprintf("measurements.shoe_size must be between %d and %d", minShoeSize, maxShoeSize))
	}

	if len(invalid) > 0 {
		return Measurements{}, fmt.Errorf("%w: %s", app.ErrInvalidInput, strings.Join(invalid, "; "))
	}

	return Measurements{Left: &left, Right: &right, ArchType: m.ArchType, ShoeSize: m.ShoeSize}, nil
}

func checkFoot(side string, foot Foot) (Foot, []string) {
	rounded := Foot{LengthCm: math.Round(foot.LengthCm*10) / 10, WidthCm: math.Round(foot.WidthCm*10) / 10}

	var invalid []string
	if rounded.LengthCm < minFootLength || rounded.LengthCm > maxFootLength {
		invalid = append(invalid, fmt.Sprintf("measurements.%s.length_cm must be between %g and %g", side, minFootLength, maxFootLength))
	}
	if rounded.WidthCm < minFootWidth || rounded.WidthCm > maxFootWidth {
		invalid = append(invalid, fmt.Sprintf("measurements.%s.width_cm must be between %g and %g", side, minFootWidth, maxFootWidth))
	}

	return rounded, invalid
}

// text trims an optional text; an empty one is no text.
func text(field string, value *string, maxLength int) (*string, error) {
	if value == nil {
		return nil, nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}

	if len([]rune(trimmed)) > maxLength {
		return nil, fmt.Errorf("%w: %s must have at most %d characters", app.ErrInvalidInput, field, maxLength)
	}

	return &trimmed, nil
}

// requiredText is a text the action can't go without, such as the reason of a rejection.
func requiredText(field string, value string, maxLength int) (string, error) {
	trimmed, err := text(field, &value, maxLength)
	if err != nil {
		return "", err
	}
	if trimmed == nil {
		return "", &app.MissingFieldsError{Fields: []string{field}}
	}

	return *trimmed, nil
}

type RequestInput struct {
	AdvertisementID string        `json:"advertisement_id"`
	Measurements    *Measurements `json:"measurements"`
	TechnicalNotes  *string       `json:"technical_notes"`
}

type AcceptInput struct {
	QuotedPrice string `json:"quoted_price"`
}

type ReasonInput struct {
	Reason string `json:"reason"`
}

type CancelInput struct {
	Reason *string `json:"reason"`
}

// ComplementInput answers the seller's question. New measurements or notes, when sent, replace the old ones.
type ComplementInput struct {
	Message        string        `json:"message"`
	Measurements   *Measurements `json:"measurements"`
	TechnicalNotes *string       `json:"technical_notes"`
}

// CustomizableAd is the ad a request is made for.
type CustomizableAd struct {
	ID                string
	SellerID          string
	Title             string
	Published         bool
	CustomMadeAllowed bool
}

type NewRequest struct {
	BuyerID         string
	AdvertisementID string
	Measurements    Measurements
	TechnicalNotes  *string
	// Notification tells the seller, once the request has its ID.
	Notification func(requestID string) notifications.NewNotification
}

// StatusChange moves a request from one status to another, if it is still in From when the change is
// written. The other fields are set only by the actions that change them.
type StatusChange struct {
	RequestID       string
	From            Status
	To              Status
	Action          Action
	ChangedBy       string
	Reason          *string
	QuotedPrice     *string
	RejectionReason *string
	Measurements    *Measurements
	TechnicalNotes  *string
	Notification    notifications.NewNotification
}

type HistoryEntity struct {
	PreviousStatus *Status
	Status         Status
	ChangedBy      *string
	Reason         *string
	CreatedAt      time.Time
}

// RequestEntity is a request with the ad it is for and both parties.
type RequestEntity struct {
	ID                     string
	BuyerID                string
	BuyerName              string
	SellerID               string
	SellerName             string
	AdvertisementID        string
	AdvertisementTitle     string
	CoverURL               *string
	OrderID                *string
	Status                 Status
	Measurements           Measurements
	TechnicalNotes         *string
	QuotedPrice            *string
	RejectionReason        *string
	AdaptationInstructions *string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// actions are what the viewer may do now: the buyer, or the seller of the ad.
func (r *RequestEntity) actions(viewerID string) []Action {
	var mine []Action
	switch viewerID {
	case r.BuyerID:
		mine = buyerActions
	case r.SellerID:
		mine = sellerActions
	}
	allowed := []Action{}
	for _, action := range mine {
		if r.Status.Allows(action) && (action != ActionCancel || r.OrderID == nil) {
			allowed = append(allowed, action)
		}
	}
	return allowed
}

func (r *RequestEntity) ToSummaryOutput() RequestSummaryOutput {
	return RequestSummaryOutput{
		ID:                 r.ID,
		Status:             string(r.Status),
		AdvertisementID:    r.AdvertisementID,
		AdvertisementTitle: r.AdvertisementTitle,
		CoverURL:           r.CoverURL,
		BuyerName:          r.BuyerName,
		SellerName:         r.SellerName,
		QuotedPrice:        r.QuotedPrice,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
	}
}

type RequestDetailEntity struct {
	RequestEntity
	History []HistoryEntity
}

func (d *RequestDetailEntity) ToOutput(viewerID string) *RequestDetailOutput {
	history := make([]HistoryOutput, len(d.History))
	for i, h := range d.History {
		var previous *string
		if h.PreviousStatus != nil {
			p := string(*h.PreviousStatus)
			previous = &p
		}
		by := "system"
		if h.ChangedBy != nil {
			switch *h.ChangedBy {
			case d.BuyerID:
				by = "buyer"
			case d.SellerID:
				by = "seller"
			}
		}
		history[i] = HistoryOutput{PreviousStatus: previous, Status: string(h.Status), ChangedBy: by, Reason: h.Reason, CreatedAt: h.CreatedAt}
	}
	actions := d.actions(viewerID)
	names := make([]string, len(actions))
	for i, action := range actions {
		names[i] = string(action)
	}
	return &RequestDetailOutput{
		RequestSummaryOutput:   d.ToSummaryOutput(),
		OrderID:                d.OrderID,
		Measurements:           d.Measurements,
		TechnicalNotes:         d.TechnicalNotes,
		RejectionReason:        d.RejectionReason,
		AdaptationInstructions: d.AdaptationInstructions,
		History:                history,
		Actions:                names,
	}
}

// Filter narrows a list of requests to one party, and optionally to one status.
type Filter struct {
	BuyerID  string
	SellerID string
	Status   Status
}

type RequestPage struct {
	Items       []*RequestEntity
	CurrentPage uint
	TotalPages  uint
	TotalItems  uint
}

func (p *RequestPage) ToOutput() *RequestPageOutput {
	items := make([]RequestSummaryOutput, len(p.Items))
	for i, request := range p.Items {
		items[i] = request.ToSummaryOutput()
	}
	return &RequestPageOutput{
		Items:       items,
		CurrentPage: p.CurrentPage,
		TotalPages:  p.TotalPages,
		TotalItems:  p.TotalItems,
		HasNext:     p.CurrentPage < p.TotalPages,
		HasPrevious: p.CurrentPage > 1,
	}
}
