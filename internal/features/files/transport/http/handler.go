package httpfiles

import (
	"errors"
	"net/http"
	"strings"

	"github.com/caslumes/solado_certo_backend/pkg/storage"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	storage storage.Storage
}

func NewHandler(storage storage.Storage) *Handler {
	return &Handler{storage: storage}
}

func (h *Handler) ServePublic(c *gin.Context) {
	key := strings.TrimPrefix(c.Param("path"), "/")

	file, err := h.storage.Open(c.Request.Context(), storage.AreaPublic, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrInvalidKey) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not open file"})
		return
	}
	defer file.Close()

	c.DataFromReader(http.StatusOK, file.Size, storage.ContentType(key), file, map[string]string{
		"Cache-Control":          "public, max-age=31536000, immutable",
		"X-Content-Type-Options": "nosniff",
	})
}
