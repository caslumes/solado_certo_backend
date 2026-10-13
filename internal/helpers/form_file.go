package helpers

import (
	"errors"
	"mime/multipart"
	"net/http"

	"github.com/caslumes/solado_certo_backend/pkg/storage"
	"github.com/gin-gonic/gin"
)

var ErrMissingFormFile = errors.New("missing form file")

// multipartOverhead leaves room for the form fields around the file.
const multipartOverhead = 1 << 20

// FormFile opens a multipart file field. The request body is capped a little above maxSize,
// so a huge upload is cut off before it is read into memory or disk (storage.ErrTooLarge).
func FormFile(c *gin.Context, field string, maxSize int64) (multipart.File, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize+multipartOverhead)

	header, err := c.FormFile(field)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, storage.ErrTooLarge
		}
		return nil, ErrMissingFormFile
	}
	return header.Open()
}
