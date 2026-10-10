package httpcatalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeService struct {
	getAdvertisements func(context.Context, catalog.GetAdvertisementsInput) (catalog.AdvertisementPage, error)
	getPurposes       func(context.Context) ([]*catalog.PurposeEntity, error)
	getConditions     func(context.Context) ([]*catalog.ConditionEntity, error)
}

func (f fakeService) GetPurposes(ctx context.Context) ([]*catalog.PurposeEntity, error) {
	return f.getPurposes(ctx)
}

func (f fakeService) GetConditions(ctx context.Context) ([]*catalog.ConditionEntity, error) {
	return f.getConditions(ctx)
}

func (f fakeService) GetAdvertisements(ctx context.Context, input catalog.GetAdvertisementsInput) (catalog.AdvertisementPage, error) {
	return f.getAdvertisements(ctx, input)
}

func (fakeService) GetAdvertisement(context.Context, string) (*catalog.AdvertisementEntity, error) {
	panic("not implemented")
}

func (fakeService) GetCategories(context.Context) (*catalog.CategoriesEntity, error) {
	panic("not implemented")
}

func getAdvertisements(t *testing.T, query string) (int, *catalog.GetAdvertisementsInput) {
	t.Helper()

	var received *catalog.GetAdvertisementsInput
	handler := NewHandler(fakeService{
		getAdvertisements: func(_ context.Context, input catalog.GetAdvertisementsInput) (catalog.AdvertisementPage, error) {
			received = &input
			return catalog.AdvertisementPage{}, nil
		},
	})
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

func TestGetAdvertisementsBindsSort(t *testing.T) {
	for _, sort := range []string{"", "relevance", "price_asc", "price_desc", "newest"} {
		t.Run(sort, func(t *testing.T) {
			code, input := getAdvertisements(t, "?sort="+sort)

			if code != http.StatusOK {
				t.Fatalf("status = %d, want %d", code, http.StatusOK)
			}
			if string(input.Sort) != sort {
				t.Fatalf("sort = %q, want %q", input.Sort, sort)
			}
		})
	}
}

func TestGetAdvertisementsRejectsInvalidQuery(t *testing.T) {
	for _, query := range []string{"?page=0", "?page=-1", "?limit=0", "?limit=101", "?price_min=abc", "?custom_made_allowed=maybe", "?sort=cheapest"} {
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

func TestGetAdvertisementsMapsInvalidInput(t *testing.T) {
	handler := NewHandler(fakeService{
		getAdvertisements: func(context.Context, catalog.GetAdvertisementsInput) (catalog.AdvertisementPage, error) {
			return catalog.AdvertisementPage{}, fmt.Errorf("%w: invalid category id", app.ErrInvalidInput)
		},
	})
	router := gin.New()
	router.GET("/catalog", handler.GetAdvertisements)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/catalog?category=palmilha", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestGetAdvertisementsBindsRepeatedPurposesAndConditions(t *testing.T) {
	code, input := getAdvertisements(t, "?purpose=prevention&purpose=correction&purpose=&condition=c-1")

	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	if want := []string{"prevention", "correction"}; !reflect.DeepEqual(input.Filters.Purposes, want) {
		t.Fatalf("purposes = %v, want %v", input.Filters.Purposes, want)
	}
	if want := []string{"c-1"}; !reflect.DeepEqual(input.Filters.Conditions, want) {
		t.Fatalf("conditions = %v, want %v", input.Filters.Conditions, want)
	}
}

func TestGetAdvertisementsLeavesPurposesAndConditionsEmptyWhenAbsent(t *testing.T) {
	_, input := getAdvertisements(t, "?purpose=&condition=")

	if input.Filters.Purposes != nil || input.Filters.Conditions != nil {
		t.Fatalf("purposes = %v, conditions = %v, want nil", input.Filters.Purposes, input.Filters.Conditions)
	}
}

func serveCatalog(handler *Handler, target string) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/catalog/purposes", handler.GetPurposes)
	router.GET("/catalog/conditions", handler.GetConditions)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func TestGetPurposesAndConditionsReturnLists(t *testing.T) {
	handler := NewHandler(fakeService{
		getPurposes: func(context.Context) ([]*catalog.PurposeEntity, error) {
			return []*catalog.PurposeEntity{{ID: "p-1", Code: "prevention", Name: "Prevenção"}}, nil
		},
		getConditions: func(context.Context) ([]*catalog.ConditionEntity, error) {
			return []*catalog.ConditionEntity{{ID: "c-1", Name: "Fascite plantar"}}, nil
		},
	})

	tests := map[string]string{
		"/catalog/purposes":   `{"purposes":[{"id":"p-1","code":"prevention","name":"Prevenção","description":null}]}`,
		"/catalog/conditions": `{"conditions":[{"id":"c-1","name":"Fascite plantar","description":null}]}`,
	}
	for target, want := range tests {
		t.Run(target, func(t *testing.T) {
			recorder := serveCatalog(handler, target)

			if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != want {
				t.Fatalf("status = %d, body = %s, want %s", recorder.Code, recorder.Body.String(), want)
			}
		})
	}
}

func TestGetPurposesAndConditionsReturnEmptyLists(t *testing.T) {
	handler := NewHandler(fakeService{
		getPurposes:   func(context.Context) ([]*catalog.PurposeEntity, error) { return nil, nil },
		getConditions: func(context.Context) ([]*catalog.ConditionEntity, error) { return nil, nil },
	})

	tests := map[string]string{
		"/catalog/purposes":   `{"purposes":[]}`,
		"/catalog/conditions": `{"conditions":[]}`,
	}
	for target, want := range tests {
		t.Run(target, func(t *testing.T) {
			if body := strings.TrimSpace(serveCatalog(handler, target).Body.String()); body != want {
				t.Fatalf("body = %s, want %s", body, want)
			}
		})
	}
}

func TestGetPurposesAndConditionsHideErrors(t *testing.T) {
	failure := errors.New("database unavailable")
	handler := NewHandler(fakeService{
		getPurposes:   func(context.Context) ([]*catalog.PurposeEntity, error) { return nil, failure },
		getConditions: func(context.Context) ([]*catalog.ConditionEntity, error) { return nil, failure },
	})

	for _, target := range []string{"/catalog/purposes", "/catalog/conditions"} {
		t.Run(target, func(t *testing.T) {
			recorder := serveCatalog(handler, target)

			if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), failure.Error()) {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
