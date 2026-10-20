package httpreviews

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/reviews"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

const (
	testBuyerID = "buyer-1"
	testOrderID = "00000000-0000-0000-0000-000000000801"
	testItemID  = "00000000-0000-0000-0000-000000000811"
	testAdID    = "00000000-0000-0000-0000-000000000601"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeService struct {
	create func(ctx context.Context, buyerID, orderID, itemID string, input reviews.ReviewInput) (*reviews.ReviewEntity, error)
	list   func(ctx context.Context, advertisementID string, page uint, limit uint) (*reviews.ReviewPage, error)
}

func (f *fakeService) CreateReview(ctx context.Context, buyerID, orderID, itemID string, input reviews.ReviewInput) (*reviews.ReviewEntity, error) {
	return f.create(ctx, buyerID, orderID, itemID, input)
}

func (f *fakeService) ListReviews(ctx context.Context, advertisementID string, page uint, limit uint) (*reviews.ReviewPage, error) {
	return f.list(ctx, advertisementID, page, limit)
}

func serve(handler *Handler, authenticated bool, method, target, body string) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/catalog/items/:advertisementID/reviews", handler.ListReviews)
	group := router.Group("", func(c *gin.Context) {
		if authenticated {
			c.Request = c.Request.WithContext(authmiddleware.WithUserID(c.Request.Context(), testBuyerID))
		}
	})
	group.POST("/orders/:orderID/items/:itemID/review", handler.CreateReview)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))
	return recorder
}

const reviewTarget = "/orders/" + testOrderID + "/items/" + testItemID + "/review"

func TestCreateReview(t *testing.T) {
	var gotBuyer, gotOrder, gotItem string
	var gotInput reviews.ReviewInput
	handler := NewHandler(&fakeService{
		create: func(_ context.Context, buyerID, orderID, itemID string, input reviews.ReviewInput) (*reviews.ReviewEntity, error) {
			gotBuyer, gotOrder, gotItem, gotInput = buyerID, orderID, itemID, input
			comfort := 2
			return &reviews.ReviewEntity{ID: "review-1", OrderItemID: itemID, ProductRating: 4, SellerRating: 5, ComfortRating: &comfort, CreatedAt: time.Now()}, nil
		},
	})

	recorder := serve(handler, true, http.MethodPost, reviewTarget, `{"product_rating":4,"seller_rating":5,"comfort_rating":2,"comment":"Boa","adaptation_feedback":"Doeu no início"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotBuyer != testBuyerID || gotOrder != testOrderID || gotItem != testItemID {
		t.Errorf("buyer, order, item = %q, %q, %q", gotBuyer, gotOrder, gotItem)
	}
	if gotInput.ProductRating != 4 || gotInput.SellerRating != 5 || *gotInput.ComfortRating != 2 || *gotInput.Comment != "Boa" || *gotInput.AdaptationFeedback != "Doeu no início" {
		t.Errorf("input = %+v", gotInput)
	}
	var body reviews.ReviewOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != "review-1" || !body.SuggestSupport {
		t.Errorf("body = %s", recorder.Body.String())
	}
}

func TestCreateReviewMapsErrors(t *testing.T) {
	for err, status := range map[error]int{
		app.ErrInvalidInput:         http.StatusBadRequest,
		app.ErrOrderNotFound:        http.StatusNotFound,
		app.ErrOrderItemNotFound:    http.StatusNotFound,
		app.ErrReviewNotAllowed:     http.StatusConflict,
		app.ErrAlreadyReviewed:      http.StatusConflict,
		errors.New("database down"): http.StatusInternalServerError,
	} {
		handler := NewHandler(&fakeService{
			create: func(context.Context, string, string, string, reviews.ReviewInput) (*reviews.ReviewEntity, error) {
				return nil, err
			},
		})
		recorder := serve(handler, true, http.MethodPost, reviewTarget, `{"product_rating":5,"seller_rating":5}`)
		if recorder.Code != status {
			t.Errorf("%v: status = %d, want %d", err, recorder.Code, status)
		}
		if strings.Contains(recorder.Body.String(), "database down") {
			t.Errorf("body leaks the internal error: %s", recorder.Body.String())
		}
	}
}

func TestCreateReviewRejectsBadRequests(t *testing.T) {
	handler := NewHandler(&fakeService{})
	if recorder := serve(handler, true, http.MethodPost, reviewTarget, `{`); recorder.Code != http.StatusBadRequest {
		t.Errorf("bad JSON: status = %d, want 400", recorder.Code)
	}
	if recorder := serve(handler, false, http.MethodPost, reviewTarget, `{}`); recorder.Code != http.StatusUnauthorized {
		t.Errorf("no user: status = %d, want 401", recorder.Code)
	}
}

func TestListReviews(t *testing.T) {
	var gotAd string
	var gotPage, gotLimit uint
	handler := NewHandler(&fakeService{
		list: func(_ context.Context, advertisementID string, page uint, limit uint) (*reviews.ReviewPage, error) {
			gotAd, gotPage, gotLimit = advertisementID, page, limit
			return &reviews.ReviewPage{
				Items:       []*reviews.PublicReviewEntity{{ID: "review-1", ReviewerName: "Maria", ProductRating: 5, SellerRating: 4}},
				CurrentPage: 1, TotalPages: 1, TotalItems: 1,
			}, nil
		},
	})

	recorder := serve(handler, false, http.MethodGet, "/catalog/items/"+testAdID+"/reviews", "")

	if recorder.Code != http.StatusOK || gotAd != testAdID || gotPage != 1 || gotLimit != 10 {
		t.Fatalf("status = %d, ad = %q, page = %d, limit = %d", recorder.Code, gotAd, gotPage, gotLimit)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	items, _ := body["items"].([]any)
	first, _ := items[0].(map[string]any)
	if first["reviewer_name"] != "Maria" || first["adaptation_feedback"] != nil || body["total_items"] != float64(1) {
		t.Errorf("body = %s", recorder.Body.String())
	}
	if _, ok := first["adaptation_feedback"]; ok {
		t.Error("public reviews must not carry the adaptation feedback")
	}
}

func TestListReviewsErrors(t *testing.T) {
	for _, query := range []string{"page=0", "limit=51"} {
		if recorder := serve(NewHandler(&fakeService{}), false, http.MethodGet, "/catalog/items/"+testAdID+"/reviews?"+query, ""); recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, recorder.Code)
		}
	}
	handler := NewHandler(&fakeService{
		list: func(context.Context, string, uint, uint) (*reviews.ReviewPage, error) {
			return nil, app.ErrAdvertisementNotFound
		},
	})
	if recorder := serve(handler, false, http.MethodGet, "/catalog/items/x/reviews", ""); recorder.Code != http.StatusNotFound {
		t.Errorf("bad id: status = %d, want 404", recorder.Code)
	}
}
