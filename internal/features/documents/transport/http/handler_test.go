package httpdocuments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/documents"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
	"github.com/gin-gonic/gin"
)

const (
	testUserID     = "user-1"
	testOrderID    = "00000000-0000-0000-0000-000000000801"
	testItemID     = "00000000-0000-0000-0000-000000000811"
	testDocumentID = "00000000-0000-0000-0000-000000000901"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeService struct {
	upload func(ctx context.Context, buyerID, orderID, itemID string, content io.Reader) (*documents.DocumentEntity, error)
	open   func(ctx context.Context, userID, id string) (*documents.DocumentFile, error)
	reject func(ctx context.Context, sellerID, orderID, id string, input documents.RejectionInput) (*documents.DocumentEntity, error)
}

func (f *fakeService) UploadPrescription(ctx context.Context, buyerID, orderID, itemID string, content io.Reader) (*documents.DocumentEntity, error) {
	return f.upload(ctx, buyerID, orderID, itemID, content)
}

func (f *fakeService) OpenDocument(ctx context.Context, userID, id string) (*documents.DocumentFile, error) {
	return f.open(ctx, userID, id)
}

func (f *fakeService) RejectPrescription(ctx context.Context, sellerID, orderID, id string, input documents.RejectionInput) (*documents.DocumentEntity, error) {
	return f.reject(ctx, sellerID, orderID, id, input)
}

func serve(handler *Handler, authenticated bool, request *http.Request) *httptest.ResponseRecorder {
	router := gin.New()
	group := router.Group("", func(c *gin.Context) {
		if authenticated {
			c.Request = c.Request.WithContext(authmiddleware.WithUserID(c.Request.Context(), testUserID))
		}
	})
	group.POST("/orders/:orderID/items/:itemID/prescription", handler.UploadPrescription)
	group.GET("/documents/:documentID", handler.OpenDocument)
	group.POST("/seller/orders/:orderID/documents/:documentID/reject", handler.RejectPrescription)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

const uploadTarget = "/orders/" + testOrderID + "/items/" + testItemID + "/prescription"

func uploadRequest(t *testing.T, field string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, "receita.pdf")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	part.Write(content)
	writer.Close()
	request := httptest.NewRequest(http.MethodPost, uploadTarget, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func approved() *documents.DocumentEntity {
	itemID := testItemID
	return &documents.DocumentEntity{ID: testDocumentID, OrderID: testOrderID, OrderItemID: &itemID, Type: documents.DocumentTypePrescription, Status: orders.DocumentStatusApproved}
}

func TestUploadPrescription(t *testing.T) {
	var gotBuyer, gotOrder, gotItem string
	var gotContent []byte
	handler := NewHandler(&fakeService{
		upload: func(_ context.Context, buyerID, orderID, itemID string, content io.Reader) (*documents.DocumentEntity, error) {
			gotBuyer, gotOrder, gotItem = buyerID, orderID, itemID
			gotContent, _ = io.ReadAll(content)
			return approved(), nil
		},
	})

	recorder := serve(handler, true, uploadRequest(t, "file", []byte("%PDF-1.4")))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotBuyer != testUserID || gotOrder != testOrderID || gotItem != testItemID || string(gotContent) != "%PDF-1.4" {
		t.Errorf("buyer, order, item, content = %q, %q, %q, %q", gotBuyer, gotOrder, gotItem, gotContent)
	}
	var output documents.DocumentOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "approved" || output.URL != "/documents/"+testDocumentID || *output.OrderItemID != testItemID {
		t.Errorf("output = %+v", output)
	}
}

func TestUploadPrescriptionErrors(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{app.ErrConsentRequired, http.StatusForbidden},
		{app.ErrOrderNotFound, http.StatusNotFound},
		{app.ErrOrderItemNotFound, http.StatusNotFound},
		{app.ErrPrescriptionNotNeeded, http.StatusConflict},
		{fmt.Errorf("%w: shipped", app.ErrInvalidStatusTransition), http.StatusConflict},
		{storage.ErrTooLarge, http.StatusRequestEntityTooLarge},
		{storage.ErrUnsupportedType, http.StatusUnsupportedMediaType},
		{fmt.Errorf("%w: the PDF is incomplete", app.ErrUnreadableDocument), http.StatusUnprocessableEntity},
		{errors.New("db down"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		handler := NewHandler(&fakeService{
			upload: func(context.Context, string, string, string, io.Reader) (*documents.DocumentEntity, error) {
				return nil, test.err
			},
		})
		if recorder := serve(handler, true, uploadRequest(t, "file", []byte("x"))); recorder.Code != test.want {
			t.Errorf("%v: status = %d, want %d", test.err, recorder.Code, test.want)
		}
	}
}

func TestUploadPrescriptionBadRequests(t *testing.T) {
	handler := NewHandler(&fakeService{})

	if recorder := serve(handler, true, uploadRequest(t, "photo", []byte("x"))); recorder.Code != http.StatusBadRequest {
		t.Errorf("wrong field: status = %d", recorder.Code)
	}
	if recorder := serve(handler, true, uploadRequest(t, "file", make([]byte, app.MaxUploadSize+2<<20))); recorder.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("huge body: status = %d", recorder.Code)
	}
	if recorder := serve(handler, false, uploadRequest(t, "file", []byte("x"))); recorder.Code != http.StatusUnauthorized {
		t.Errorf("no user: status = %d", recorder.Code)
	}
}

func TestOpenDocument(t *testing.T) {
	var gotUser, gotID string
	handler := NewHandler(&fakeService{
		open: func(_ context.Context, userID, id string) (*documents.DocumentFile, error) {
			gotUser, gotID = userID, id
			content := "%PDF-1.4"
			return &documents.DocumentFile{
				File:        &storage.File{ReadCloser: io.NopCloser(strings.NewReader(content)), Size: int64(len(content))},
				ContentType: "application/pdf",
				Name:        "prescription-SC-1.pdf",
			}, nil
		},
	})

	recorder := serve(handler, true, httptest.NewRequest(http.MethodGet, "/documents/"+testDocumentID, nil))

	if recorder.Code != http.StatusOK || recorder.Body.String() != "%PDF-1.4" {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotUser != testUserID || gotID != testDocumentID {
		t.Errorf("user, id = %q, %q", gotUser, gotID)
	}
	headers := recorder.Header()
	if headers.Get("Content-Type") != "application/pdf" || headers.Get("Cache-Control") != "private, no-store" ||
		headers.Get("Content-Disposition") != `inline; filename="prescription-SC-1.pdf"` {
		t.Errorf("headers = %v", headers)
	}
}

func TestOpenDocumentNotFound(t *testing.T) {
	handler := NewHandler(&fakeService{
		open: func(context.Context, string, string) (*documents.DocumentFile, error) {
			return nil, app.ErrDocumentNotFound
		},
	})

	if recorder := serve(handler, true, httptest.NewRequest(http.MethodGet, "/documents/"+testDocumentID, nil)); recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", recorder.Code)
	}
}

func TestRejectPrescription(t *testing.T) {
	var gotSeller, gotOrder, gotID string
	var gotInput documents.RejectionInput
	handler := NewHandler(&fakeService{
		reject: func(_ context.Context, sellerID, orderID, id string, input documents.RejectionInput) (*documents.DocumentEntity, error) {
			gotSeller, gotOrder, gotID, gotInput = sellerID, orderID, id, input
			document := approved()
			document.Status, document.RejectionReason = orders.DocumentStatusRejected, &input.Reason
			return document, nil
		},
	})
	target := "/seller/orders/" + testOrderID + "/documents/" + testDocumentID + "/reject"

	recorder := serve(handler, true, httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{"reason":"Receita vencida"}`)))

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"rejection_reason":"Receita vencida"`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotSeller != testUserID || gotOrder != testOrderID || gotID != testDocumentID || gotInput.Reason != "Receita vencida" {
		t.Errorf("seller, order, id, input = %q, %q, %q, %+v", gotSeller, gotOrder, gotID, gotInput)
	}

	if recorder := serve(handler, true, httptest.NewRequest(http.MethodPost, target, strings.NewReader("{"))); recorder.Code != http.StatusBadRequest {
		t.Errorf("bad JSON: status = %d, want 400", recorder.Code)
	}
	missing := NewHandler(&fakeService{
		reject: func(context.Context, string, string, string, documents.RejectionInput) (*documents.DocumentEntity, error) {
			return nil, &app.MissingFieldsError{Fields: []string{"reason"}}
		},
	})
	recorder = serve(missing, true, httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{}`)))
	if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), `"fields":["reason"]`) {
		t.Errorf("no reason: status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
