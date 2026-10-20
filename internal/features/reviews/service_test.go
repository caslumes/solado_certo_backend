package reviews

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

const (
	testBuyerID = "buyer-1"
	testOrderID = "00000000-0000-0000-0000-000000000801"
	testItemID  = "00000000-0000-0000-0000-000000000811"
	testAdID    = "00000000-0000-0000-0000-000000000601"
)

type fakeRepository struct {
	item      *ReviewedItem
	getErr    error
	created   []NewReview
	createErr error
	total     uint
	reviews   []*PublicReviewEntity
	gotLimit  uint
	gotOffset uint
	listErr   error
}

func (f *fakeRepository) GetOrderItem(_ context.Context, orderID string, itemID string) (*ReviewedItem, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	item := *f.item
	if itemID != item.ID {
		item.ID = ""
	}
	return &item, nil
}

func (f *fakeRepository) CreateReview(_ context.Context, review NewReview) (*ReviewEntity, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, review)
	return &ReviewEntity{ID: "review-1", OrderItemID: review.OrderItemID, ProductRating: review.ProductRating, SellerRating: review.SellerRating, ComfortRating: review.ComfortRating}, nil
}

func (f *fakeRepository) CountReviews(context.Context, string) (uint, error) {
	return f.total, f.listErr
}

func (f *fakeRepository) GetReviews(_ context.Context, _ string, limit uint, offset uint) ([]*PublicReviewEntity, error) {
	f.gotLimit, f.gotOffset = limit, offset
	return f.reviews, f.listErr
}

func delivered() *fakeRepository {
	return &fakeRepository{item: &ReviewedItem{ID: testItemID, OrderID: testOrderID, BuyerID: testBuyerID, OrderStatus: orderStatusDelivered}}
}

func ptr[T any](v T) *T {
	return &v
}

func TestCreateReview(t *testing.T) {
	r := delivered()
	input := ReviewInput{ProductRating: 5, SellerRating: 4, ComfortRating: ptr(4), Comment: ptr("  Ótima palmilha  "), AdaptationFeedback: ptr("Adaptei em uma semana.")}

	review, err := NewService(r).CreateReview(context.Background(), testBuyerID, testOrderID, testItemID, input)

	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if review.ID != "review-1" || review.ToOutput().SuggestSupport {
		t.Errorf("review = %+v", review)
	}
	got := r.created[0]
	if got.OrderItemID != testItemID || got.ReviewerID != testBuyerID || got.ProductRating != 5 || got.SellerRating != 4 ||
		*got.ComfortRating != 4 || *got.Comment != "Ótima palmilha" || *got.AdaptationFeedback != "Adaptei em uma semana." {
		t.Errorf("created = %+v", got)
	}
}

func TestCreateReviewValidatesTheInput(t *testing.T) {
	long := strings.Repeat("a", maxTextLength+1)
	for name, input := range map[string]ReviewInput{
		"no ratings":       {},
		"product too high": {ProductRating: 6, SellerRating: 5},
		"seller too low":   {ProductRating: 5, SellerRating: 0},
		"comfort too high": {ProductRating: 5, SellerRating: 5, ComfortRating: ptr(6)},
		"long comment":     {ProductRating: 5, SellerRating: 5, Comment: &long},
		"long feedback":    {ProductRating: 5, SellerRating: 5, AdaptationFeedback: &long},
	} {
		r := delivered()
		if _, err := NewService(r).CreateReview(context.Background(), testBuyerID, testOrderID, testItemID, input); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("%s: error = %v, want ErrInvalidInput", name, err)
		}
		if len(r.created) != 0 {
			t.Errorf("%s: created = %+v", name, r.created)
		}
	}
}

func TestCreateReviewEmptyTextsAreNone(t *testing.T) {
	r := delivered()
	input := ReviewInput{ProductRating: 3, SellerRating: 3, Comment: ptr("   "), AdaptationFeedback: ptr("")}

	if _, err := NewService(r).CreateReview(context.Background(), testBuyerID, testOrderID, testItemID, input); err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if r.created[0].Comment != nil || r.created[0].AdaptationFeedback != nil || r.created[0].ComfortRating != nil {
		t.Errorf("created = %+v, want no texts and no comfort", r.created[0])
	}
}

func TestCreateReviewRejects(t *testing.T) {
	valid := ReviewInput{ProductRating: 5, SellerRating: 5}
	tests := []struct {
		name    string
		setup   func(*fakeRepository)
		buyer   string
		order   string
		item    string
		wantErr error
	}{
		{"bad order id", nil, testBuyerID, "x", testItemID, app.ErrOrderNotFound},
		{"bad item id", nil, testBuyerID, testOrderID, "x", app.ErrOrderItemNotFound},
		{"unknown order", func(r *fakeRepository) { r.getErr = app.ErrOrderNotFound }, testBuyerID, testOrderID, testItemID, app.ErrOrderNotFound},
		{"another buyer", nil, "buyer-2", testOrderID, testItemID, app.ErrOrderNotFound},
		{"another buyer, unknown item", nil, "buyer-2", testOrderID, "00000000-0000-0000-0000-000000000899", app.ErrOrderNotFound},
		{"unknown item", nil, testBuyerID, testOrderID, "00000000-0000-0000-0000-000000000899", app.ErrOrderItemNotFound},
		{"not delivered", func(r *fakeRepository) { r.item.OrderStatus = "shipped" }, testBuyerID, testOrderID, testItemID, app.ErrReviewNotAllowed},
		{"already reviewed", func(r *fakeRepository) { r.createErr = app.ErrAlreadyReviewed }, testBuyerID, testOrderID, testItemID, app.ErrAlreadyReviewed},
		{"repository failure", func(r *fakeRepository) { r.getErr = errors.New("db down") }, testBuyerID, testOrderID, testItemID, nil},
	}
	for _, test := range tests {
		r := delivered()
		if test.setup != nil {
			test.setup(r)
		}
		_, err := NewService(r).CreateReview(context.Background(), test.buyer, test.order, test.item, valid)
		if test.wantErr == nil && err == nil || test.wantErr != nil && !errors.Is(err, test.wantErr) {
			t.Errorf("%s: error = %v, want %v", test.name, err, test.wantErr)
		}
		if len(r.created) != 0 {
			t.Errorf("%s: created = %+v", test.name, r.created)
		}
	}
}

func TestSuggestSupportWhenComfortIsLow(t *testing.T) {
	for comfort, want := range map[int]bool{1: true, 2: true, 3: false, 5: false} {
		review := ReviewEntity{ComfortRating: ptr(comfort)}
		if got := review.ToOutput().SuggestSupport; got != want {
			t.Errorf("comfort %d: suggest support = %v, want %v", comfort, got, want)
		}
	}
	if (&ReviewEntity{}).ToOutput().SuggestSupport {
		t.Error("no comfort rating: suggest support = true")
	}
}

func TestListReviews(t *testing.T) {
	r := &fakeRepository{total: 12, reviews: []*PublicReviewEntity{{ID: "review-1", ReviewerName: "Maria", ProductRating: 5}}}

	page, err := NewService(r).ListReviews(context.Background(), testAdID, 2, 5)

	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if r.gotLimit != 5 || r.gotOffset != 5 || page.TotalPages != 3 || page.TotalItems != 12 {
		t.Errorf("limit, offset = %d, %d; page = %+v", r.gotLimit, r.gotOffset, page)
	}
	output := page.ToOutput()
	if !output.HasNext || !output.HasPrevious || output.Items[0].ReviewerName != "Maria" {
		t.Errorf("output = %+v", output)
	}
}

func TestListReviewsRejects(t *testing.T) {
	s := NewService(&fakeRepository{})
	if _, err := s.ListReviews(context.Background(), "x", 1, 10); !errors.Is(err, app.ErrAdvertisementNotFound) {
		t.Errorf("bad id: error = %v", err)
	}
	if _, err := s.ListReviews(context.Background(), testAdID, 1, maxReviewsPerPage+1); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("limit too big: error = %v", err)
	}
	page, err := s.ListReviews(context.Background(), testAdID, 0, 0)
	if err != nil || page.CurrentPage != 1 {
		t.Errorf("defaults: page = %+v, error = %v", page, err)
	}

	r := &fakeRepository{listErr: errors.New("db down")}
	if _, err := NewService(r).ListReviews(context.Background(), testAdID, 1, 10); !errors.Is(err, r.listErr) {
		t.Errorf("repository failure: error = %v", err)
	}
}
