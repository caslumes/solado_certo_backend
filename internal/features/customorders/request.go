package customorders

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
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

type Action string

const (
	ActionReview      Action = "review"
	ActionAccept      Action = "accept"
	ActionReject      Action = "reject"
	ActionRequestInfo Action = "request_info"
	ActionComplement  Action = "complement"
	ActionCancel      Action = "cancel"
	ActionReport      Action = "report"
	ActionCheckout    Action = "checkout"
	ActionSendPhoto   Action = "send_photo"
)

type transition struct {
	from []Status
	to   Status
}

var transitions = map[Action]transition{
	ActionReview:      {from: []Status{StatusRequested}, to: StatusUnderReview},
	ActionAccept:      {from: []Status{StatusRequested, StatusUnderReview}, to: StatusApproved},
	ActionReject:      {from: []Status{StatusRequested, StatusUnderReview, StatusNeedsInfo}, to: StatusRejected},
	ActionRequestInfo: {from: []Status{StatusRequested, StatusUnderReview}, to: StatusNeedsInfo},
	ActionComplement:  {from: []Status{StatusNeedsInfo}, to: StatusUnderReview},
	ActionCancel:      {from: []Status{StatusRequested, StatusUnderReview, StatusNeedsInfo, StatusApproved}, to: StatusCancelled},
	ActionReport:      {from: []Status{StatusInProduction}, to: StatusCompleted},
}

var (
	sellerActions = []Action{ActionReview, ActionAccept, ActionReject, ActionRequestInfo, ActionReport}
	buyerActions  = []Action{ActionComplement, ActionCancel, ActionCheckout, ActionSendPhoto}
	photoStatuses = []Status{StatusRequested, StatusUnderReview, StatusNeedsInfo}
)

func (s Status) Allows(action Action) bool {
	return slices.Contains(transitions[action].from, s)
}

func (s Status) TakesPhoto() bool {
	return slices.Contains(photoStatuses, s)
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

	maxNotesLength                      = 2000
	maxMessageLength                    = 1000
	maxInstructionsLength               = 4000
	maxQuotedPrice        helpers.Cents = 99_999_999_99
)

type Foot struct {
	LengthCm float64 `json:"length_cm"`
	WidthCm  float64 `json:"width_cm"`
}

type Measurements struct {
	Left     *Foot    `json:"left"`
	Right    *Foot    `json:"right"`
	ArchType ArchType `json:"arch_type"`
	ShoeSize int      `json:"shoe_size"`
}

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

type CheckoutInput struct {
	AddressID        string `json:"address_id"`
	ShippingOptionID string `json:"shipping_option_id"`
}

type Checkout struct {
	RequestID      string
	BuyerID        string
	IdempotencyKey string
	Plan           *orders.CheckoutPlan
}

type PhotoChange struct {
	RequestID string
	BuyerID   string
	Key       string
}

type PhotoFile struct {
	*storage.File
	ContentType string
	Name        string
}

type ComplementInput struct {
	Message        string        `json:"message"`
	Measurements   *Measurements `json:"measurements"`
	TechnicalNotes *string       `json:"technical_notes"`
}

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
	Notification    func(requestID string) notifications.NewNotification
}

type StatusChange struct {
	RequestID              string
	From                   Status
	To                     Status
	Action                 Action
	ChangedBy              string
	Reason                 *string
	QuotedPrice            *string
	RejectionReason        *string
	Measurements           *Measurements
	TechnicalNotes         *string
	AdaptationInstructions *string
	ReportKey              *string
	Notification           notifications.NewNotification
}

type HistoryEntity struct {
	PreviousStatus *Status
	Status         Status
	ChangedBy      *string
	Reason         *string
	CreatedAt      time.Time
}

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
	FootPhotoKey           *string
	FittingReport          *FittingReportEntity
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type FittingReportEntity struct {
	ID        string
	CreatedAt time.Time
}

func (r *RequestEntity) can(action Action) bool {
	switch action {
	case ActionCheckout:
		return r.Status == StatusApproved && r.OrderID == nil
	case ActionSendPhoto:
		return r.Status.TakesPhoto()
	case ActionCancel:
		return r.Status.Allows(action) && r.OrderID == nil
	}
	return r.Status.Allows(action)
}

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
		if r.can(action) {
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
	var photoURL *string
	if d.FootPhotoKey != nil {
		path := "/custom-orders/" + d.ID + "/photo"
		if viewerID == d.SellerID {
			path = "/seller" + path
		}
		photoURL = &path
	}
	var report *FittingReportOutput
	if d.FittingReport != nil {
		report = &FittingReportOutput{ID: d.FittingReport.ID, URL: orders.DocumentPath(d.FittingReport.ID), CreatedAt: d.FittingReport.CreatedAt}
	}
	return &RequestDetailOutput{
		RequestSummaryOutput:   d.ToSummaryOutput(),
		OrderID:                d.OrderID,
		Measurements:           d.Measurements,
		TechnicalNotes:         d.TechnicalNotes,
		RejectionReason:        d.RejectionReason,
		AdaptationInstructions: d.AdaptationInstructions,
		FootPhotoURL:           photoURL,
		FittingReport:          report,
		History:                history,
		Actions:                names,
	}
}

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
