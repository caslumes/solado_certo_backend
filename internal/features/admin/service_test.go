package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
)

const (
	adminID  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	userID   = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	otherID  = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	adID     = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	sellerID = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
)

var now = time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC)

type fakeRepository struct {
	users       map[string]*UserEntity
	ads         map[string]*AdEntity
	moderations []UserModeration
	removals    []AdRemoval
	beforeBlock enum.ProfileStatus
	userFilter  UserFilter
	orderFilter OrderFilter
	payFilter   PaymentFilter
	auditFilter AuditFilter
	period      Period
	limit       uint
	offset      uint
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		users: map[string]*UserEntity{
			adminID: {ID: adminID, Status: enum.ProfileStatusActive, Roles: []string{"admin", "buyer"}},
			userID:  {ID: userID, Status: enum.ProfileStatusActive, Roles: []string{"buyer"}},
			otherID: {ID: otherID, Status: enum.ProfileStatusPending, Roles: []string{"buyer", "seller"}},
		},
		ads: map[string]*AdEntity{adID: {ID: adID, Title: "Palmilha Conforto", Status: "published", SellerID: sellerID}},
	}
}

func (f *fakeRepository) ListUsers(_ context.Context, filter UserFilter, limit uint, offset uint) ([]*UserEntity, uint, error) {
	f.userFilter, f.limit, f.offset = filter, limit, offset
	return []*UserEntity{f.users[userID]}, 41, nil
}

func (f *fakeRepository) GetUser(_ context.Context, id string) (*UserEntity, error) {
	user, ok := f.users[id]
	if !ok {
		return nil, app.ErrUserNotFound
	}
	copied := *user
	return &copied, nil
}

func (f *fakeRepository) StatusBeforeBlock(context.Context, string) (enum.ProfileStatus, error) {
	return f.beforeBlock, nil
}

func (f *fakeRepository) ModerateUser(_ context.Context, moderation UserModeration) error {
	user := f.users[moderation.UserID]
	if user.Status != moderation.FromStatus {
		return app.ErrInvalidStatusTransition
	}
	user.Status, user.SuspendedUntil = moderation.ToStatus, moderation.SuspendedUntil
	f.moderations = append(f.moderations, moderation)
	return nil
}

func (f *fakeRepository) ListAds(_ context.Context, _ AdFilter, limit uint, offset uint) ([]*AdEntity, uint, error) {
	return nil, 0, nil
}

func (f *fakeRepository) GetAd(_ context.Context, id string) (*AdEntity, error) {
	ad, ok := f.ads[id]
	if !ok {
		return nil, app.ErrAdvertisementNotFound
	}
	copied := *ad
	return &copied, nil
}

func (f *fakeRepository) RemoveAd(_ context.Context, removal AdRemoval) error {
	ad := f.ads[removal.AdID]
	ad.Status, ad.RemovedAt = "removed", &now
	f.removals = append(f.removals, removal)
	return nil
}

func (f *fakeRepository) ListOrders(_ context.Context, filter OrderFilter, _ uint, _ uint) ([]*OrderEntity, uint, error) {
	f.orderFilter = filter
	return nil, 0, nil
}

func (f *fakeRepository) ListPayments(_ context.Context, filter PaymentFilter, _ uint, _ uint) ([]*PaymentEntity, uint, error) {
	f.payFilter = filter
	return nil, 0, nil
}

func (f *fakeRepository) ListAuditLogs(_ context.Context, filter AuditFilter, _ uint, _ uint) ([]*AuditEntity, uint, error) {
	f.auditFilter = filter
	return nil, 0, nil
}

func (f *fakeRepository) GetIndicators(_ context.Context, period Period, _ time.Time) (*Indicators, error) {
	f.period = period
	return &Indicators{}, nil
}

func newTestService() (*service, *fakeRepository) {
	r := newFakeRepository()
	s := NewService(r)
	s.now = func() time.Time { return now }
	return s, r
}

func TestListUsersPagesAndValidates(t *testing.T) {
	s, r := newTestService()

	page, err := s.ListUsers(context.Background(), UserQuery{Search: "  ana ", Status: "suspended", Role: "seller", Paging: Paging{Page: 3, Limit: 10}})
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if r.userFilter.Search != "ana" || r.userFilter.Status != "suspended" || r.userFilter.Role != "seller" || !r.userFilter.Now.Equal(now) {
		t.Fatalf("filter = %+v", r.userFilter)
	}
	if r.limit != 10 || r.offset != 20 || page.TotalPages != 5 || page.CurrentPage != 3 {
		t.Fatalf("limit = %d, offset = %d, page = %+v", r.limit, r.offset, page)
	}

	for name, query := range map[string]UserQuery{
		"status": {Status: "deleted"},
		"role":   {Role: "root"},
		"limit":  {Paging: Paging{Limit: 101}},
		"search": {Search: strings.Repeat("a", 101)},
	} {
		if _, err := s.ListUsers(context.Background(), query); !errors.Is(err, app.ErrInvalidInput) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}

func TestBlockUserRevokesSessionsAndRecordsReason(t *testing.T) {
	s, r := newTestService()

	user, err := s.BlockUser(context.Background(), adminID, userID, ModerationInput{Reason: "  Fraude em pagamentos  "})
	if err != nil {
		t.Fatalf("BlockUser: %v", err)
	}
	if user.Status != enum.ProfileStatusBlocked {
		t.Fatalf("status = %s", user.Status)
	}
	m := r.moderations[0]
	if m.Action != ActionBlockUser || m.AdminID != adminID || !m.RevokeSessions || m.Details["reason"] != "Fraude em pagamentos" || m.Details["previous_status"] != enum.ProfileStatusActive {
		t.Fatalf("moderation = %+v", m)
	}

	if _, err := s.BlockUser(context.Background(), adminID, userID, ModerationInput{Reason: "De novo, por engano"}); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("blocked twice: err = %v", err)
	}
}

func TestModerationRefusals(t *testing.T) {
	s, r := newTestService()
	r.users[otherID].Roles = []string{"admin"}
	reason := ModerationInput{Reason: "Motivo suficiente"}

	tests := map[string]struct {
		userID string
		input  ModerationInput
		want   error
	}{
		"own account":   {adminID, reason, app.ErrForbidden},
		"another admin": {otherID, reason, app.ErrForbidden},
		"unknown":       {"ffffffff-ffff-ffff-ffff-ffffffffffff", reason, app.ErrUserNotFound},
		"not a uuid":    {"x", reason, app.ErrUserNotFound},
		"no reason":     {userID, ModerationInput{Reason: " "}, app.ErrMissingRequiredFields},
		"short reason":  {userID, ModerationInput{Reason: "spam"}, app.ErrInvalidInput},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := s.BlockUser(context.Background(), adminID, tt.userID, tt.input); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
	if len(r.moderations) != 0 {
		t.Fatalf("moderations = %+v", r.moderations)
	}
}

func TestSuspendUser(t *testing.T) {
	s, r := newTestService()
	until := now.Add(7 * 24 * time.Hour)

	user, err := s.SuspendUser(context.Background(), adminID, userID, ModerationInput{Reason: "Ofensas no suporte", Until: &until})
	if err != nil {
		t.Fatalf("SuspendUser: %v", err)
	}
	if user.StatusAt(now) != enum.ProfileStatusSuspended || user.Status != enum.ProfileStatusActive || !user.SuspendedUntil.Equal(until) {
		t.Fatalf("user = %+v", user)
	}
	if out := user.ToOutput(now); out.Status != enum.ProfileStatusSuspended || out.SuspendedUntil == nil {
		t.Fatalf("output = %+v", out)
	}
	if out := user.ToOutput(until.Add(time.Second)); out.Status != enum.ProfileStatusActive || out.SuspendedUntil != nil {
		t.Fatalf("output after the suspension = %+v", out)
	}
	if m := r.moderations[0]; m.Action != ActionSuspendUser || !m.RevokeSessions || m.Details["until"] != until {
		t.Fatalf("moderation = %+v", m)
	}

	past := now.Add(-time.Minute)
	tooLong := now.Add(400 * 24 * time.Hour)
	for name, tt := range map[string]struct {
		input ModerationInput
		want  error
	}{
		"no until":  {ModerationInput{Reason: "Ofensas no suporte"}, app.ErrMissingRequiredFields},
		"past":      {ModerationInput{Reason: "Ofensas no suporte", Until: &past}, app.ErrInvalidInput},
		"too long":  {ModerationInput{Reason: "Ofensas no suporte", Until: &tooLong}, app.ErrInvalidInput},
		"no reason": {ModerationInput{Until: &until}, app.ErrMissingRequiredFields},
	} {
		if _, err := s.SuspendUser(context.Background(), adminID, userID, tt.input); !errors.Is(err, tt.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, tt.want)
		}
	}

	r.users[userID].Status = enum.ProfileStatusBlocked
	if _, err := s.SuspendUser(context.Background(), adminID, userID, ModerationInput{Reason: "Ofensas no suporte", Until: &until}); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("blocked: err = %v", err)
	}
}

func TestReactivateUser(t *testing.T) {
	s, r := newTestService()
	input := ModerationInput{Reason: "Contestação aceita"}

	if _, err := s.ReactivateUser(context.Background(), adminID, userID, input); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("active: err = %v", err)
	}

	if _, err := s.BlockUser(context.Background(), adminID, otherID, ModerationInput{Reason: "Anúncios falsos"}); err != nil {
		t.Fatalf("BlockUser: %v", err)
	}
	r.beforeBlock = enum.ProfileStatusPending
	user, err := s.ReactivateUser(context.Background(), adminID, otherID, input)
	if err != nil {
		t.Fatalf("ReactivateUser: %v", err)
	}
	if user.Status != enum.ProfileStatusPending {
		t.Fatalf("status = %s, want the status before the block", user.Status)
	}
	if m := r.moderations[1]; m.Action != ActionReactivateUser || m.RevokeSessions || m.Details["previous_status"] != enum.ProfileStatusBlocked {
		t.Fatalf("moderation = %+v", m)
	}

	until := now.Add(time.Hour)
	r.users[userID].SuspendedUntil = &until
	user, err = s.ReactivateUser(context.Background(), adminID, userID, input)
	if err != nil {
		t.Fatalf("ReactivateUser suspended: %v", err)
	}
	if user.Status != enum.ProfileStatusActive || user.SuspendedUntil != nil {
		t.Fatalf("user = %+v", user)
	}

	r.users[userID].Status, r.beforeBlock = enum.ProfileStatusBlocked, ""
	if user, err = s.ReactivateUser(context.Background(), adminID, userID, input); err != nil || user.Status != enum.ProfileStatusActive {
		t.Fatalf("no block record: user = %+v, err = %v", user, err)
	}
}

func TestRemoveAdNotifiesSeller(t *testing.T) {
	s, r := newTestService()

	ad, err := s.RemoveAd(context.Background(), adminID, adID, "Promete cura de doenças")
	if err != nil {
		t.Fatalf("RemoveAd: %v", err)
	}
	if ad.Status != "removed" || ad.RemovedAt == nil {
		t.Fatalf("ad = %+v", ad)
	}
	removal := r.removals[0]
	if removal.FromStatus != "published" || removal.Reason != "Promete cura de doenças" || removal.AdminID != adminID {
		t.Fatalf("removal = %+v", removal)
	}
	n := removal.Notification
	if n.UserID != sellerID || n.Type != notifications.TypeAdvertisementRemoved || !strings.Contains(n.Body, "Promete cura de doenças") || n.Data["advertisement_id"] != adID {
		t.Fatalf("notification = %+v", n)
	}

	if _, err := s.RemoveAd(context.Background(), adminID, adID, "Promete cura de doenças"); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("removed twice: err = %v", err)
	}
	if _, err := s.RemoveAd(context.Background(), adminID, "x", "Promete cura de doenças"); !errors.Is(err, app.ErrAdvertisementNotFound) {
		t.Fatalf("not a uuid: err = %v", err)
	}
	if _, err := s.RemoveAd(context.Background(), adminID, adID, ""); !errors.Is(err, app.ErrMissingRequiredFields) {
		t.Fatalf("no reason: err = %v", err)
	}
}

func TestListsValidateFilters(t *testing.T) {
	s, r := newTestService()
	ctx := context.Background()

	if _, err := s.ListOrders(ctx, OrderQuery{Status: "paid", From: "2026-11-01", To: "2026-11-01"}); err != nil {
		t.Fatalf("ListOrders: %v", err)
	}
	if r.orderFilter.Since == nil || r.orderFilter.Until == nil || r.orderFilter.Until.Sub(*r.orderFilter.Since) != 24*time.Hour {
		t.Fatalf("filter = %+v", r.orderFilter)
	}
	if _, err := s.ListPayments(ctx, PaymentQuery{Method: "pix", RefundGivenUp: true}); err != nil || !r.payFilter.RefundGivenUp || r.payFilter.Method != "pix" {
		t.Fatalf("ListPayments: filter = %+v, err = %v", r.payFilter, err)
	}
	if _, err := s.ListAuditLogs(ctx, AuditQuery{Entity: "user", UserID: adminID, Action: "admin."}); err != nil || r.auditFilter.Action != "admin." {
		t.Fatalf("ListAuditLogs: filter = %+v, err = %v", r.auditFilter, err)
	}

	invalid := map[string]func() error{
		"order status": func() error { _, err := s.ListOrders(ctx, OrderQuery{Status: "lost"}); return err },
		"order dates": func() error {
			_, err := s.ListOrders(ctx, OrderQuery{From: "2026-11-02", To: "2026-11-01"})
			return err
		},
		"order format":   func() error { _, err := s.ListOrders(ctx, OrderQuery{From: "02/11/2026"}); return err },
		"payment status": func() error { _, err := s.ListPayments(ctx, PaymentQuery{Status: "paid"}); return err },
		"payment method": func() error { _, err := s.ListPayments(ctx, PaymentQuery{Method: "cash"}); return err },
		"ad status":      func() error { _, err := s.ListAds(ctx, AdQuery{Status: "sold"}); return err },
		"ad seller":      func() error { _, err := s.ListAds(ctx, AdQuery{SellerID: "x"}); return err },
		"audit entity":   func() error { _, err := s.ListAuditLogs(ctx, AuditQuery{EntityID: "x"}); return err },
		"audit user":     func() error { _, err := s.ListAuditLogs(ctx, AuditQuery{UserID: "x"}); return err },
	}
	for name, call := range invalid {
		if err := call(); !errors.Is(err, app.ErrInvalidInput) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}

func TestGetIndicatorsPeriod(t *testing.T) {
	s, r := newTestService()

	indicators, err := s.GetIndicators(context.Background(), "", "")
	if err != nil {
		t.Fatalf("GetIndicators: %v", err)
	}
	out := indicators.ToOutput()
	if out.From != "2026-10-04" || out.To != "2026-11-02" || r.period.Until.Sub(r.period.From) != 30*24*time.Hour {
		t.Fatalf("output = %s..%s, period = %+v", out.From, out.To, r.period)
	}
	if out.OrdersByStatus["paid"] != 0 || out.PaymentsByStatus["captured"].Amount != "0.00" || len(out.OrdersByStatus) != len(orderStatuses) {
		t.Fatalf("output = %+v", out)
	}

	indicators, err = s.GetIndicators(context.Background(), "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatalf("GetIndicators october: %v", err)
	}
	if out := indicators.ToOutput(); out.From != "2026-10-01" || out.To != "2026-10-31" {
		t.Fatalf("october = %s..%s", out.From, out.To)
	}

	for _, period := range [][2]string{{"2026-11-02", "2026-11-01"}, {"2025-01-01", "2026-11-01"}, {"ontem", ""}, {"2026-12-01", ""}} {
		if _, err := s.GetIndicators(context.Background(), period[0], period[1]); !errors.Is(err, app.ErrInvalidInput) {
			t.Fatalf("%v: err = %v", period, err)
		}
	}
}
