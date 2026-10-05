package httpcatalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	httpconstants "github.com/caslumes/solado_certo_backend/internal/app/constants/http"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
)

var (
	errInvalidPage  = errors.New("page must be a positive integer")
	errInvalidLimit = errors.New("limit must be between 1 and 100")
)

type getAdvertisementsQuery struct {
	Page  uint
	Limit uint
}

type Handler struct {
	getAdvertisements catalog.GetAdvertisementsUseCase
	getCategories     catalog.GetCategoriesUseCase
}

func NewHandler(getAdvertisements catalog.GetAdvertisementsUseCase, getCategories catalog.GetCategoriesUseCase) *Handler {
	return &Handler{
		getAdvertisements: getAdvertisements,
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
		Page:  query.Page,
		Limit: query.Limit,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(ap.ToOutput())
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
	page := uint64(1)
	if value := query.Get("page"); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 0)
		if err != nil || parsed == 0 {
			return getAdvertisementsQuery{}, errInvalidPage
		}
		page = parsed
	}

	limit := uint64(10)
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 0)
		if err != nil || parsed == 0 || parsed > 100 {
			return getAdvertisementsQuery{}, errInvalidLimit
		}
		limit = parsed
	}

	return getAdvertisementsQuery{
		Page:  uint(page),
		Limit: uint(limit),
	}, nil
}
