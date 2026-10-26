package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/customorders"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

const unknownID = "00000000-0000-0000-0000-000000000000"

type fixture struct {
	pool   *pgxpool.Pool
	r      *postgresRepository
	buyer  testutil.User
	seller testutil.User
	ad     testutil.Ad
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testutil.DB(t)
	f := fixture{pool: pool, r: NewPostgresRepository(pool), buyer: testutil.InsertUser(t, pool), seller: testutil.InsertUser(t, pool)}
	category := testutil.InsertCategory(t, pool, nil)
	f.ad = testutil.InsertAd(t, pool, testutil.Ad{SellerID: f.seller.ID, CategoryID: category.ID, Title: "Palmilha sob medida", Stock: 1, CustomMadeAllowed: true})
	return f
}

func measurements(shoeSize int) customorders.Measurements {
	return customorders.Measurements{
		Left:     &customorders.Foot{LengthCm: 25.1, WidthCm: 9.5},
		Right:    &customorders.Foot{LengthCm: 25.3, WidthCm: 9.6},
		ArchType: customorders.ArchTypeFlat,
		ShoeSize: shoeSize,
	}
}

func (f fixture) create(t *testing.T, buyerID string) string {
	t.Helper()
	notes := "Esporão no calcanhar"
	id, err := f.r.CreateRequest(context.Background(), customorders.NewRequest{
		BuyerID: buyerID, AdvertisementID: f.ad.ID, Measurements: measurements(39), TechnicalNotes: &notes,
		Notification: func(id string) notifications.NewNotification {
			return notifications.CustomOrderRequested(f.seller.ID, id, f.ad.Title)
		},
	})
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	return id
}

func (f fixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func (f fixture) change(id string, from customorders.Status, action customorders.Action, to customorders.Status) customorders.StatusChange {
	return customorders.StatusChange{
		RequestID: id, From: from, To: to, Action: action, ChangedBy: f.seller.ID,
		Notification: notifications.CustomOrderUnderReview(f.buyer.ID, id, f.ad.Title),
	}
}

func TestGetAdvertisement(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	ad, err := f.r.GetAdvertisement(ctx, f.ad.ID)
	if err != nil {
		t.Fatalf("GetAdvertisement: %v", err)
	}
	if ad.SellerID != f.seller.ID || ad.Title != "Palmilha sob medida" || !ad.Published || !ad.CustomMadeAllowed {
		t.Errorf("ad = %+v", ad)
	}

	if _, err := f.pool.Exec(ctx, "UPDATE advertisements SET status = 'removed', removed_at = now() WHERE id = $1", f.ad.ID); err != nil {
		t.Fatalf("remove ad: %v", err)
	}
	if ad, _ := f.r.GetAdvertisement(ctx, f.ad.ID); ad.Published {
		t.Error("a removed ad is published")
	}
	if _, err := f.r.GetAdvertisement(ctx, unknownID); !errors.Is(err, app.ErrAdvertisementNotFound) {
		t.Errorf("unknown ad err = %v", err)
	}
}

func TestCreateAndGetRequest(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, "INSERT INTO advertisement_images (advertisement_id, url, position, is_cover) VALUES ($1, '/files/public/ads/a.jpg', 1, true)", f.ad.ID); err != nil {
		t.Fatalf("insert image: %v", err)
	}

	id := f.create(t, f.buyer.ID)
	request, err := f.r.GetRequest(ctx, id)

	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if request.BuyerID != f.buyer.ID || request.BuyerName != f.buyer.Name || request.SellerID != f.seller.ID || request.SellerName != f.seller.Name ||
		request.AdvertisementID != f.ad.ID || request.AdvertisementTitle != "Palmilha sob medida" || *request.CoverURL != "/files/public/ads/a.jpg" ||
		request.Status != customorders.StatusRequested || *request.TechnicalNotes != "Esporão no calcanhar" || request.OrderID != nil || request.QuotedPrice != nil {
		t.Errorf("request = %+v", request.RequestEntity)
	}
	if m := request.Measurements; m.Left.LengthCm != 25.1 || m.Right.WidthCm != 9.6 || m.ArchType != customorders.ArchTypeFlat || m.ShoeSize != 39 {
		t.Errorf("measurements = %+v", m)
	}
	if len(request.History) != 1 || request.History[0].PreviousStatus != nil || request.History[0].Status != customorders.StatusRequested || *request.History[0].ChangedBy != f.buyer.ID {
		t.Errorf("history = %+v", request.History)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity = 'custom_order' AND entity_id = $1 AND action = 'custom_order.requested'", id); n != 1 {
		t.Errorf("audit rows = %d", n)
	}
	if n := f.count(t, "SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'custom_order_requested' AND data->>'custom_order_id' = $2", f.seller.ID, id); n != 1 {
		t.Errorf("seller notifications = %d", n)
	}

	if _, err := f.r.GetRequest(ctx, unknownID); !errors.Is(err, app.ErrCustomOrderNotFound) {
		t.Errorf("unknown request err = %v", err)
	}
}

func TestChangeStatus(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := f.create(t, f.buyer.ID)

	question := "Qual a altura do arco?"
	ask := f.change(id, customorders.StatusRequested, customorders.ActionRequestInfo, customorders.StatusNeedsInfo)
	ask.Reason = &question
	if err := f.r.ChangeStatus(ctx, ask); err != nil {
		t.Fatalf("request info: %v", err)
	}

	answer, notes, newMeasurements := "Arco alto", "Uso diário", measurements(40)
	complement := f.change(id, customorders.StatusNeedsInfo, customorders.ActionComplement, customorders.StatusUnderReview)
	complement.ChangedBy, complement.Reason, complement.Measurements, complement.TechnicalNotes = f.buyer.ID, &answer, &newMeasurements, &notes
	if err := f.r.ChangeStatus(ctx, complement); err != nil {
		t.Fatalf("complement: %v", err)
	}

	price := "189.90"
	accept := f.change(id, customorders.StatusUnderReview, customorders.ActionAccept, customorders.StatusApproved)
	accept.QuotedPrice = &price
	if err := f.r.ChangeStatus(ctx, accept); err != nil {
		t.Fatalf("accept: %v", err)
	}

	request, err := f.r.GetRequest(ctx, id)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if request.Status != customorders.StatusApproved || *request.QuotedPrice != "189.90" || request.Measurements.ShoeSize != 40 ||
		*request.TechnicalNotes != "Uso diário" || request.RejectionReason != nil || !request.UpdatedAt.After(request.CreatedAt) {
		t.Errorf("request = %+v", request.RequestEntity)
	}
	if len(request.History) != 4 {
		t.Fatalf("history = %+v", request.History)
	}
	if h := request.History[1]; *h.PreviousStatus != customorders.StatusRequested || h.Status != customorders.StatusNeedsInfo || *h.Reason != question || *h.ChangedBy != f.seller.ID {
		t.Errorf("history[1] = %+v", h)
	}
	if h := request.History[2]; *h.Reason != answer || *h.ChangedBy != f.buyer.ID {
		t.Errorf("history[2] = %+v", h)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'custom_order.accept' AND context->>'quoted_price' = '189.90'", id); n != 1 {
		t.Errorf("accept audit rows = %d", n)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'custom_order.complement' AND (context->>'measurements_changed')::boolean", id); n != 1 {
		t.Errorf("complement audit rows = %d", n)
	}
	if n := f.count(t, "SELECT count(*) FROM notifications WHERE data->>'custom_order_id' = $1", id); n != 4 {
		t.Errorf("notifications = %d, want 4", n)
	}
}

func TestChangeStatusFromAnOldStatus(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := f.create(t, f.buyer.ID)

	err := f.r.ChangeStatus(ctx, f.change(id, customorders.StatusUnderReview, customorders.ActionAccept, customorders.StatusApproved))

	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("err = %v", err)
	}
	if n := f.count(t, "SELECT count(*) FROM custom_order_request_history WHERE request_id = $1", id); n != 1 {
		t.Errorf("history rows = %d", n)
	}
	if err := f.r.ChangeStatus(ctx, f.change(unknownID, customorders.StatusRequested, customorders.ActionReview, customorders.StatusUnderReview)); !errors.Is(err, app.ErrCustomOrderNotFound) {
		t.Errorf("unknown request err = %v", err)
	}
}

// TestConcurrentDecisions has the seller accept and reject at the same time: only one applies.
func TestConcurrentDecisions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := f.create(t, f.buyer.ID)

	price, reason := "100.00", "Sem material"
	accept := f.change(id, customorders.StatusRequested, customorders.ActionAccept, customorders.StatusApproved)
	accept.QuotedPrice = &price
	reject := f.change(id, customorders.StatusRequested, customorders.ActionReject, customorders.StatusRejected)
	reject.Reason, reject.RejectionReason = &reason, &reason

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, change := range []customorders.StatusChange{accept, reject} {
		wg.Go(func() { errs[i] = f.r.ChangeStatus(ctx, change) })
	}
	wg.Wait()

	failed := 0
	for _, err := range errs {
		if errors.Is(err, app.ErrInvalidStatusTransition) {
			failed++
		} else if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	}
	if failed != 1 {
		t.Errorf("errors = %v, want exactly one refused", errs)
	}
	if n := f.count(t, "SELECT count(*) FROM custom_order_request_history WHERE request_id = $1", id); n != 2 {
		t.Errorf("history rows = %d, want 2", n)
	}
}

func TestListRequests(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	otherBuyer := testutil.InsertUser(t, f.pool)
	otherSeller := testutil.InsertUser(t, f.pool)
	category := testutil.InsertCategory(t, f.pool, nil)
	otherAd := testutil.InsertAd(t, f.pool, testutil.Ad{SellerID: otherSeller.ID, CategoryID: category.ID, Stock: 1, CustomMadeAllowed: true})

	first := f.create(t, f.buyer.ID)
	f.create(t, f.buyer.ID)
	third := f.create(t, otherBuyer.ID)
	other := fixture{pool: f.pool, r: f.r, seller: otherSeller, ad: otherAd}
	other.create(t, f.buyer.ID)
	if err := f.r.ChangeStatus(ctx, f.change(first, customorders.StatusRequested, customorders.ActionReview, customorders.StatusUnderReview)); err != nil {
		t.Fatalf("review: %v", err)
	}

	for name, test := range map[string]struct {
		filter customorders.Filter
		total  uint
	}{
		"buyer":            {customorders.Filter{BuyerID: f.buyer.ID}, 3},
		"buyer by status":  {customorders.Filter{BuyerID: f.buyer.ID, Status: customorders.StatusUnderReview}, 1},
		"seller":           {customorders.Filter{SellerID: f.seller.ID}, 3},
		"seller by status": {customorders.Filter{SellerID: f.seller.ID, Status: customorders.StatusRequested}, 2},
		"other seller":     {customorders.Filter{SellerID: otherSeller.ID}, 1},
	} {
		t.Run(name, func(t *testing.T) {
			total, err := f.r.CountRequests(ctx, test.filter)
			if err != nil || total != test.total {
				t.Errorf("CountRequests = %d, %v, want %d", total, err, test.total)
			}
			items, err := f.r.GetRequests(ctx, test.filter, 10, 0)
			if err != nil || uint(len(items)) != test.total {
				t.Errorf("GetRequests = %d items, %v", len(items), err)
			}
		})
	}

	items, err := f.r.GetRequests(ctx, customorders.Filter{SellerID: f.seller.ID}, 2, 0)
	if err != nil {
		t.Fatalf("GetRequests: %v", err)
	}
	if len(items) != 2 || items[0].ID != first || items[1].ID != third {
		t.Errorf("order = %s, %s; want %s, %s (the last changed first)", items[0].ID, items[1].ID, first, third)
	}
	if page2, _ := f.r.GetRequests(ctx, customorders.Filter{SellerID: f.seller.ID}, 2, 2); len(page2) != 1 {
		t.Errorf("page 2 = %d items", len(page2))
	}
}
