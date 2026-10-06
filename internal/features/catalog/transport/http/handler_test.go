package httpcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type getAdvertisementsUseCaseFunc func(context.Context, catalog.GetAdvertisementsInput) (catalog.AdvertisementPage, error)

func (f getAdvertisementsUseCaseFunc) GetAdvertisements(ctx context.Context, input catalog.GetAdvertisementsInput) (catalog.AdvertisementPage, error) {
	return f(ctx, input)
}

func getAdvertisements(t *testing.T, query string) (int, *catalog.GetAdvertisementsInput) {
	t.Helper()

	var received *catalog.GetAdvertisementsInput
	handler := NewHandler(
		getAdvertisementsUseCaseFunc(func(_ context.Context, input catalog.GetAdvertisementsInput) (catalog.AdvertisementPage, error) {
			received = &input
			return catalog.AdvertisementPage{}, nil
		}),
		nil,
		nil,
	)
	router := gin.New()
	router.GET("/catalog", handler.GetAdvertisements)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/catalog"+query, nil))

	return recorder.Code, received
}

func TestGetAdvertisementsDefaults(t *testing.T) {
	code, input := getAdvertisements(t, "")

	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	if input.Page != 1 || input.Limit != 10 {
		t.Fatalf("page, limit = %d, %d, want 1, 10", input.Page, input.Limit)
	}
	if input.Filters.Category != nil || input.Filters.PriceMin != nil || input.Filters.PrescriptionRequired != nil {
		t.Fatalf("absent filters should be nil: %#v", input.Filters)
	}
}

func TestGetAdvertisementsBindsFilters(t *testing.T) {
	code, input := getAdvertisements(t, "?page=2&limit=5&category=palmilha&price_min=10.5&prescription_required=true")

	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	f := input.Filters
	if input.Page != 2 || input.Limit != 5 {
		t.Fatalf("page, limit = %d, %d, want 2, 5", input.Page, input.Limit)
	}
	if f.Category == nil || *f.Category != "palmilha" {
		t.Fatalf("category = %v, want palmilha", f.Category)
	}
	if f.PriceMin == nil || *f.PriceMin != 10.5 {
		t.Fatalf("price_min = %v, want 10.5", f.PriceMin)
	}
	if f.PrescriptionRequired == nil || !*f.PrescriptionRequired {
		t.Fatalf("prescription_required = %v, want true", f.PrescriptionRequired)
	}
}

func TestGetAdvertisementsRejectsInvalidQuery(t *testing.T) {
	for _, query := range []string{"?page=0", "?page=-1", "?limit=0", "?limit=101", "?price_min=abc", "?custom_made_allowed=maybe"} {
		t.Run(query, func(t *testing.T) {
			code, input := getAdvertisements(t, query)

			if code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", code, http.StatusBadRequest)
			}
			if input != nil {
				t.Fatal("use case was called for an invalid query")
			}
		})
	}
}

func TestGetAdvertisementsTreatsEmptyValuesAsAbsent(t *testing.T) {
	code, input := getAdvertisements(t, "?page=&category=&price_min=&prescription_required=")

	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	f := input.Filters
	if input.Page != 1 || f.Category != nil || f.PriceMin != nil || f.PrescriptionRequired != nil {
		t.Fatalf("empty params should fall back to defaults/nil: page=%d filters=%#v", input.Page, f)
	}
}
