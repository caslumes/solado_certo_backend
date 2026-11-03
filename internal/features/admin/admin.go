package admin

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
)

var userStatuses = []enum.ProfileStatus{
	enum.ProfileStatusActive, enum.ProfileStatusPending, enum.ProfileStatusInactive, enum.ProfileStatusBlocked, enum.ProfileStatusSuspended,
}

var adStatuses = []string{"draft", "published", "paused", "archived", "removed"}

var orderStatuses = []string{"awaiting_payment", "paid", "processing", "shipped", "delivered", "cancelled", "returned"}

var paymentStatuses = []string{"pending", "authorized", "captured", "failed", "refunded", "cancelled"}

var paymentMethods = []string{"pix", "boleto", "credit_card"}

const (
	ActionBlockUser      = "admin.user_blocked"
	ActionSuspendUser    = "admin.user_suspended"
	ActionReactivateUser = "admin.user_reactivated"
	ActionRemoveAd       = "admin.ad_removed"
)

const (
	minReasonLength   = 5
	maxReasonLength   = 500
	maxSuspension     = 365 * 24 * time.Hour
	defaultPerPage    = 20
	maxPerPage        = 100
	maxSearchLength   = 100
	defaultPeriodDays = 30
	maxPeriodDays     = 366
)

var brasilia = func() *time.Location {
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*60*60)
	}
	return location
}()

type Paging struct {
	Page  uint
	Limit uint
}

func (p Paging) check() (Paging, error) {
	if p.Page == 0 {
		p.Page = 1
	}
	if p.Limit == 0 {
		p.Limit = defaultPerPage
	}
	if p.Limit > maxPerPage {
		return Paging{}, fmt.Errorf("%w: limit must be at most %d", app.ErrInvalidInput, maxPerPage)
	}
	return p, nil
}

func (p Paging) offset() uint {
	return (p.Page - 1) * p.Limit
}

type Page[T any] struct {
	Items       []T
	CurrentPage uint
	TotalPages  uint
	TotalItems  uint
}

func newPage[T any](items []T, paging Paging, total uint) *Page[T] {
	return &Page[T]{Items: items, CurrentPage: paging.Page, TotalPages: (total + paging.Limit - 1) / paging.Limit, TotalItems: total}
}

type Period struct {
	From  time.Time
	Until time.Time
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

func parseRange(from string, to string) (*time.Time, *time.Time, error) {
	since, err := parseDay("from", from)
	if err != nil {
		return nil, nil, err
	}
	until, err := parseDay("to", to)
	if err != nil {
		return nil, nil, err
	}
	if until != nil {
		next := until.AddDate(0, 0, 1)
		until = &next
	}
	if since != nil && until != nil && !since.Before(*until) {
		return nil, nil, fmt.Errorf("%w: from must not be after to", app.ErrInvalidInput)
	}
	return since, until, nil
}

func indicatorPeriod(from string, to string, now time.Time) (Period, error) {
	since, until, err := parseRange(from, to)
	if err != nil {
		return Period{}, err
	}
	if until == nil {
		today := now.In(brasilia)
		next := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, brasilia).AddDate(0, 0, 1)
		until = &next
	}
	if since == nil {
		start := until.AddDate(0, 0, -defaultPeriodDays)
		since = &start
	}
	if !since.Before(*until) {
		return Period{}, fmt.Errorf("%w: from must not be after to", app.ErrInvalidInput)
	}
	if until.Sub(*since) > maxPeriodDays*24*time.Hour {
		return Period{}, fmt.Errorf("%w: the period must have at most %d days", app.ErrInvalidInput, maxPeriodDays)
	}
	return Period{From: *since, Until: *until}, nil
}

func checkOneOf[T ~string](name string, value T, allowed []T) error {
	if value == "" || slices.Contains(allowed, value) {
		return nil
	}
	names := make([]string, len(allowed))
	for i, option := range allowed {
		names[i] = string(option)
	}
	return fmt.Errorf("%w: %s must be one of %s", app.ErrInvalidInput, name, strings.Join(names, ", "))
}

func checkSearch(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len([]rune(trimmed)) > maxSearchLength {
		return "", fmt.Errorf("%w: q must have at most %d characters", app.ErrInvalidInput, maxSearchLength)
	}
	return trimmed, nil
}

func checkReason(reason string) (string, error) {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return "", &app.MissingFieldsError{Fields: []string{"reason"}}
	}
	if n := len([]rune(trimmed)); n < minReasonLength || n > maxReasonLength {
		return "", fmt.Errorf("%w: reason must have between %d and %d characters", app.ErrInvalidInput, minReasonLength, maxReasonLength)
	}
	return trimmed, nil
}

type UserQuery struct {
	Search string
	Status enum.ProfileStatus
	Role   string
	Paging
}

type UserFilter struct {
	Search string
	Status enum.ProfileStatus
	Role   string
	Now    time.Time
}

type UserEntity struct {
	ID             string
	Name           string
	Email          string
	Phone          string
	Status         enum.ProfileStatus
	SuspendedUntil *time.Time
	Roles          []string
	CreatedAt      time.Time
}

func (u *UserEntity) StatusAt(now time.Time) enum.ProfileStatus {
	if u.Status != enum.ProfileStatusBlocked && u.SuspendedUntil != nil && now.Before(*u.SuspendedUntil) {
		return enum.ProfileStatusSuspended
	}
	return u.Status
}

type ModerationInput struct {
	Reason string     `json:"reason"`
	Until  *time.Time `json:"until"`
}

type UserModeration struct {
	UserID         string
	AdminID        string
	FromStatus     enum.ProfileStatus
	ToStatus       enum.ProfileStatus
	SuspendedUntil *time.Time
	RevokeSessions bool
	Action         string
	Details        map[string]any
}

type AdQuery struct {
	Search   string
	Status   string
	SellerID string
	Paging
}

type AdFilter struct {
	Search   string
	Status   string
	SellerID string
}

type AdEntity struct {
	ID         string
	Title      string
	Status     string
	Price      string
	Stock      int
	SellerID   string
	SellerName string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	RemovedAt  *time.Time
}

type AdRemoval struct {
	AdID         string
	AdminID      string
	Reason       string
	FromStatus   string
	Notification notifications.NewNotification
}

type OrderQuery struct {
	Search string
	Status string
	From   string
	To     string
	Paging
}

type OrderFilter struct {
	Search string
	Status string
	Since  *time.Time
	Until  *time.Time
}

type OrderEntity struct {
	ID          string
	Number      string
	Status      string
	Total       string
	BuyerID     string
	BuyerName   string
	BuyerEmail  string
	SellerID    *string
	SellerName  *string
	CreatedAt   time.Time
	PaidAt      *time.Time
	DeliveredAt *time.Time
}

type PaymentQuery struct {
	Status        string
	Method        string
	RefundGivenUp bool
	From          string
	To            string
	Paging
}

type PaymentFilter struct {
	Status        string
	Method        string
	RefundGivenUp bool
	Since         *time.Time
	Until         *time.Time
}

type PaymentEntity struct {
	ID            string
	OrderID       string
	OrderNumber   string
	Gateway       string
	Method        string
	Status        string
	Amount        string
	FailureReason *string
	RefundError   *string
	RefundGivenUp bool
	CreatedAt     time.Time
	CapturedAt    *time.Time
	RefundedAt    *time.Time
}

type AuditQuery struct {
	Entity   string
	EntityID string
	UserID   string
	Action   string
	From     string
	To       string
	Paging
}

type AuditFilter struct {
	Entity   string
	EntityID string
	UserID   string
	Action   string
	Since    *time.Time
	Until    *time.Time
}

type AuditEntity struct {
	ID        string
	UserID    *string
	UserName  *string
	Entity    string
	EntityID  string
	Action    string
	Context   json.RawMessage
	CreatedAt time.Time
}

type Amount struct {
	Count  uint
	Amount string
}

type Indicators struct {
	Period           Period
	NewUsers         uint
	BlockedUsers     uint
	SuspendedUsers   uint
	NewAds           uint
	PublishedAds     uint
	Orders           uint
	OrdersByStatus   map[string]uint
	PaidAmount       string
	PaidOrders       uint
	PaymentsByStatus map[string]Amount
	RefundsGivenUp   uint
	TicketsOpened    uint
	TicketsResolved  uint
	OpenTickets      uint
}

func isAdminRole(roles []string) bool {
	return slices.Contains(roles, string(enum.RoleAdmin))
}
