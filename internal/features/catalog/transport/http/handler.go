package httpcatalog

import (
	"encoding/json"
	"net/http"
	"strconv"

	httpconstants "github.com/caslumes/solado_certo_backend/internal/app/constants/http"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
)

type getAdvertisementsQuery struct {
	Cursor *string
	Limit *uint
}

type Handler struct {
	getAdvertisements catalog.GetAdvertisementsUseCase
}

func NewHandler(getAdvertisements catalog.GetAdvertisementsUseCase) *Handler {
	return &Handler{
		getAdvertisements: getAdvertisements,
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
		Cursor: query.Cursor,
		Limit:  query.Limit,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(ap.ToOutput())
}

func parsePaginationParams(r *http.Request) (getAdvertisementsQuery, error) {
	query := r.URL.Query()
	cursor := query.Get("cursor")
	limit := query.Get("limit")
	
	var cursorPtr *string
	if cursor != "" {
		cursorPtr = &cursor
	}

	var limitInt int
	if limit != "" {
		var err error
		limitInt, err = strconv.Atoi(limit)
		if err != nil {
			return getAdvertisementsQuery{}, err
		}
		limitUint := uint(limitInt) 
		return getAdvertisementsQuery{
			Cursor: cursorPtr,
			Limit:  &limitUint,
		}, nil
	}


	return getAdvertisementsQuery{
		Cursor: cursorPtr,
		Limit:  nil,
	}, nil
}