package httpcatalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/caslumes/solado_certo_backend/internal/app"
	httpconstants "github.com/caslumes/solado_certo_backend/internal/app/constants/http"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

var (
	errInvalidPage  = errors.New("page must be a positive integer")
	errInvalidLimit = errors.New("limit must be between 1 and 100")
)

type getAdvertisementsQuery struct {
	Page    uint
	Limit   uint
	Filters *catalog.AdvertisementFilters
}

type Handler struct {
	getAdvertisements catalog.GetAdvertisementsUseCase
	getAdvertisement    catalog.GetAdvertisementUseCase
	getCategories     catalog.GetCategoriesUseCase
}

func NewHandler(getAdvertisements catalog.GetAdvertisementsUseCase, getAdvertisement catalog.GetAdvertisementUseCase, getCategories catalog.GetCategoriesUseCase) *Handler {
	return &Handler{
		getAdvertisements: getAdvertisements,
		getAdvertisement: getAdvertisement,
		getCategories:     getCategories,
	}
}

func (h *Handler) GetAdvertisements(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)

	query, err := parsePaginationParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ap, err := h.getAdvertisements.GetAdvertisements(r.Context(), catalog.GetAdvertisementsInput{
		Page:    query.Page,
		Limit:   query.Limit,
		Filters: query.Filters,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(ap.ToOutput())
}

func (h *Handler) GetAdvertisement(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)

	id := r.PathValue("advertisementID")
	if id == "" {
		http.Error(w, "advertisementID is required", http.StatusBadRequest)
		return
	}

	ad, err := h.getAdvertisement.GetAdvertisement(r.Context(), id)
	if err != nil {
		if errors.Is(err, app.ErrAdvertisementNotFound) {
			http.Error(w, "Advertisement not found", http.StatusNotFound)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(ad.ToOutput())
}

func (h *Handler) GetCategories(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconstants.ContentTypeHeader, httpconstants.MimeTypeApplicationJSON)

	categories, err := h.getCategories.GetCategories(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(categories.ToOutput())
}

func parsePaginationParams(r *http.Request) (getAdvertisementsQuery, error) {
	query := r.URL.Query()

	page, err := getUintQueryParam(query, "page", helpers.UintPtr(1))
	if err != nil {
		return getAdvertisementsQuery{}, errInvalidPage
	}

	limit, err := getUintQueryParam(query, "limit", helpers.UintPtr(10))
	if err != nil {
		return getAdvertisementsQuery{}, errInvalidLimit
	}

	priceMin, err := getFloatQueryParam(query, "price_min", nil)
	if err != nil {
		return getAdvertisementsQuery{}, errors.New("price_min must be a valid float")
	}

	priceMax, err := getFloatQueryParam(query, "price_max", nil)
	if err != nil {
		return getAdvertisementsQuery{}, errors.New("price_max must be a valid float")
	}

	prescriptionRequired, err := getBoolQueryParam(query, "prescription_required", nil)
	if err != nil {
		return getAdvertisementsQuery{}, errors.New("prescription_required must be a valid boolean")
	}

	customMadeAllowed, err := getBoolQueryParam(query, "custom_made_allowed", nil)
	if err != nil {
		return getAdvertisementsQuery{}, errors.New("custom_made_allowed must be a valid boolean")
	}

	filters := &catalog.AdvertisementFilters{
		Category:             getStringQueryParam(query, "category", nil),
		PriceMin:             priceMin,
		PriceMax:             priceMax,
		PrescriptionRequired: prescriptionRequired,
		CustomMadeAllowed:    customMadeAllowed,
		Side:                 getStringQueryParam(query, "side", nil),
		Size:                 getStringQueryParam(query, "size", nil),
		Material:             getStringQueryParam(query, "material", nil),
		Rigidity:             getStringQueryParam(query, "rigidity", nil),
		UsageIndication:      getStringQueryParam(query, "usage_indication", nil),
	}

	return getAdvertisementsQuery{
		Page:    uint(*page),
		Limit:   uint(*limit),
		Filters: filters,
	}, nil
}

func getUintQueryParam(query url.Values, key string, defaultValue *uint) (*uint, error) {
	value := query.Get(key)
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.ParseUint(value, 10, 0)
	if err != nil || parsed == 0 {
		return nil, errors.New(key + " must be a positive integer")
	}

	uintParsed := uint(parsed)

	return &uintParsed, nil
}

func getFloatQueryParam(query url.Values, key string, defaultValue *float64) (*float64, error) {
	value := query.Get(key)
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, errors.New(key + " must be a valid float")
	}

	return &parsed, nil
}

func getBoolQueryParam(query url.Values, key string, defaultValue *bool) (*bool, error) {
	value := query.Get(key)
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, errors.New(key + " must be a valid boolean")
	}

	return &parsed, nil
}

func getStringQueryParam(query url.Values, key string, defaultValue *string) *string {
	value := query.Get(key)
	if value == "" {
		return defaultValue
	}

	return &value
}
