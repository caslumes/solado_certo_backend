package httpsupport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/support"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
	"github.com/gin-gonic/gin"
)

const userID = "user-1"

type fakeService struct {
	create func(context.Context, string, support.TicketInput) (*support.TicketDetailEntity, error)
	list   func(context.Context, string, support.Status, uint, uint) (*support.TicketPage, error)
	get    func(context.Context, string, string) (*support.TicketDetailEntity, error)
	add    func(context.Context, string, string, string, io.Reader) (*support.TicketDetailEntity, error)
	open   func(context.Context, string, string, string) (*support.AttachmentFile, error)
	queue  func(context.Context, string, support.QueueQuery, uint, uint) (*support.TicketPage, error)
	assign func(context.Context, string, string) (*support.TicketDetailEntity, error)
	close  func(context.Context, string, string, string) (*support.TicketDetailEntity, error)
}

func (f fakeService) CreateTicket(ctx context.Context, userID string, input support.TicketInput) (*support.TicketDetailEntity, error) {
	return f.create(ctx, userID, input)
}

func (f fakeService) ListTickets(ctx context.Context, userID string, status support.Status, page uint, limit uint) (*support.TicketPage, error) {
	return f.list(ctx, userID, status, page, limit)
}

func (f fakeService) GetTicket(ctx context.Context, userID string, id string) (*support.TicketDetailEntity, error) {
	return f.get(ctx, userID, id)
}

func (f fakeService) AddMessage(ctx context.Context, userID string, id string, message string, attachment io.Reader) (*support.TicketDetailEntity, error) {
	return f.add(ctx, userID, id, message, attachment)
}

func (f fakeService) OpenAttachment(ctx context.Context, userID string, id string, messageID string) (*support.AttachmentFile, error) {
	return f.open(ctx, userID, id, messageID)
}

func (f fakeService) ListQueue(ctx context.Context, adminID string, query support.QueueQuery, page uint, limit uint) (*support.TicketPage, error) {
	return f.queue(ctx, adminID, query, page, limit)
}

func (f fakeService) AssignTicket(ctx context.Context, adminID string, id string) (*support.TicketDetailEntity, error) {
	return f.assign(ctx, adminID, id)
}

func (f fakeService) CloseTicket(ctx context.Context, adminID string, id string, resolution string) (*support.TicketDetailEntity, error) {
	return f.close(ctx, adminID, id, resolution)
}

func init() {
	gin.SetMode(gin.TestMode)
}

func serve(service fakeService, request *http.Request) *httptest.ResponseRecorder {
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(authmiddleware.WithUserID(c.Request.Context(), userID))
	})
	router.POST("/support/tickets", handler.CreateTicket)
	router.GET("/support/tickets", handler.ListTickets)
	router.GET("/support/tickets/:ticketID", handler.GetTicket)
	router.POST("/support/tickets/:ticketID/messages", handler.AddMessage)
	router.GET("/support/tickets/:ticketID/messages/:messageID/attachment", handler.OpenAttachment)
	router.GET("/admin/support/tickets", handler.ListQueue)
	router.POST("/admin/support/tickets/:ticketID/assign", handler.AssignTicket)
	router.POST("/admin/support/tickets/:ticketID/close", handler.CloseTicket)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func ticket() *support.TicketDetailEntity {
	return &support.TicketDetailEntity{TicketEntity: support.TicketEntity{
		ID: "ticket-1", Category: support.CategoryOther, Status: support.StatusOpen, Subject: "Dúvida", OpenedBy: userID, OpenedAt: time.Now(), UpdatedAt: time.Now(),
	}}
}

func TestCreateTicketDecodesInput(t *testing.T) {
	var received support.TicketInput
	service := fakeService{create: func(_ context.Context, id string, input support.TicketInput) (*support.TicketDetailEntity, error) {
		if id != userID {
			t.Errorf("user = %q", id)
		}
		received = input
		return ticket(), nil
	}}

	body := `{"category":"order","subject":"Produto errado","description":"Veio o tamanho errado.","order_id":"o-1"}`
	recorder := serve(service, httptest.NewRequest(http.MethodPost, "/support/tickets", strings.NewReader(body)))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if received.Category != "order" || received.OrderID == nil || *received.OrderID != "o-1" {
		t.Fatalf("input = %+v", received)
	}
	if !strings.Contains(recorder.Body.String(), `"opened_by_me":true`) || !strings.Contains(recorder.Body.String(), `"can_reply":true`) {
		t.Fatalf("body = %s", recorder.Body)
	}
}

func TestCreateTicketMapsErrors(t *testing.T) {
	tests := map[string]struct {
		body string
		err  error
		want int
	}{
		"malformed":      {"{", nil, http.StatusBadRequest},
		"missing fields": {"{}", &app.MissingFieldsError{Fields: []string{"subject"}}, http.StatusUnprocessableEntity},
		"invalid":        {"{}", app.ErrInvalidInput, http.StatusBadRequest},
		"order":          {"{}", app.ErrOrderNotFound, http.StatusNotFound},
		"unexpected":     {"{}", errors.New("boom"), http.StatusInternalServerError},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			service := fakeService{create: func(context.Context, string, support.TicketInput) (*support.TicketDetailEntity, error) {
				return nil, tt.err
			}}

			recorder := serve(service, httptest.NewRequest(http.MethodPost, "/support/tickets", strings.NewReader(tt.body)))

			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}
		})
	}
}

func TestListTicketsPassesQuery(t *testing.T) {
	service := fakeService{list: func(_ context.Context, _ string, status support.Status, page uint, limit uint) (*support.TicketPage, error) {
		if status != support.StatusOpen || page != 2 || limit != 5 {
			t.Errorf("status = %s, page = %d, limit = %d", status, page, limit)
		}
		return &support.TicketPage{Items: []*support.TicketEntity{&ticket().TicketEntity}, CurrentPage: 2, TotalPages: 3, TotalItems: 11}, nil
	}}

	recorder := serve(service, httptest.NewRequest(http.MethodGet, "/support/tickets?status=open&page=2&limit=5", nil))

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"has_next":true`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	recorder = serve(service, httptest.NewRequest(http.MethodGet, "/support/tickets?limit=500", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("limit 500: status = %d", recorder.Code)
	}
}

func TestGetTicketMapsNotFound(t *testing.T) {
	service := fakeService{get: func(_ context.Context, _ string, id string) (*support.TicketDetailEntity, error) {
		if id != "ticket-9" {
			t.Errorf("id = %q", id)
		}
		return nil, app.ErrTicketNotFound
	}}

	recorder := serve(service, httptest.NewRequest(http.MethodGet, "/support/tickets/ticket-9", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func multipartMessage(t *testing.T, message string, file []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("message", message)
	if file != nil {
		part, err := writer.CreateFormFile("file", "foto.png")
		if err != nil {
			t.Fatal(err)
		}
		part.Write(file)
	}
	writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/support/tickets/ticket-1/messages", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestAddMessageAcceptsMultipartWithFile(t *testing.T) {
	var gotMessage string
	var gotFile []byte
	service := fakeService{add: func(_ context.Context, _ string, _ string, message string, attachment io.Reader) (*support.TicketDetailEntity, error) {
		gotMessage = message
		if attachment != nil {
			gotFile, _ = io.ReadAll(attachment)
		}
		return ticket(), nil
	}}

	recorder := serve(service, multipartMessage(t, "Segue a foto", []byte("png-bytes")))

	if recorder.Code != http.StatusCreated || gotMessage != "Segue a foto" || string(gotFile) != "png-bytes" {
		t.Fatalf("status = %d, message = %q, file = %q", recorder.Code, gotMessage, gotFile)
	}
}

func TestAddMessageAcceptsMultipartOrJSONWithoutFile(t *testing.T) {
	for name, request := range map[string]*http.Request{
		"multipart": multipartMessage(t, "Olá", nil),
		"json":      httptest.NewRequest(http.MethodPost, "/support/tickets/ticket-1/messages", strings.NewReader(`{"message":"Olá"}`)),
	} {
		t.Run(name, func(t *testing.T) {
			service := fakeService{add: func(_ context.Context, _ string, _ string, message string, attachment io.Reader) (*support.TicketDetailEntity, error) {
				if message != "Olá" || attachment != nil {
					t.Errorf("message = %q, attachment = %v", message, attachment)
				}
				return ticket(), nil
			}}

			if recorder := serve(service, request); recorder.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
		})
	}
}

func TestAddMessageMapsErrors(t *testing.T) {
	tests := map[string]struct {
		err  error
		want int
	}{
		"closed":     {app.ErrInvalidStatusTransition, http.StatusConflict},
		"type":       {storage.ErrUnsupportedType, http.StatusUnsupportedMediaType},
		"size":       {storage.ErrTooLarge, http.StatusRequestEntityTooLarge},
		"unreadable": {app.ErrUnreadableDocument, http.StatusUnprocessableEntity},
		"not found":  {app.ErrTicketNotFound, http.StatusNotFound},
		"no message": {&app.MissingFieldsError{Fields: []string{"message"}}, http.StatusUnprocessableEntity},
		"unexpected": {errors.New("boom"), http.StatusInternalServerError},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			service := fakeService{add: func(context.Context, string, string, string, io.Reader) (*support.TicketDetailEntity, error) {
				return nil, tt.err
			}}

			if recorder := serve(service, multipartMessage(t, "Olá", nil)); recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}
		})
	}

	recorder := serve(fakeService{}, httptest.NewRequest(http.MethodPost, "/support/tickets/ticket-1/messages", strings.NewReader("{")))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed JSON: status = %d", recorder.Code)
	}
}

func TestOpenAttachmentServesPrivately(t *testing.T) {
	service := fakeService{open: func(_ context.Context, _ string, id string, messageID string) (*support.AttachmentFile, error) {
		if id != "ticket-1" || messageID != "m-1" {
			t.Errorf("id = %q, message = %q", id, messageID)
		}
		return &support.AttachmentFile{File: &storage.File{ReadCloser: io.NopCloser(strings.NewReader("%PDF")), Size: 4}, ContentType: "application/pdf", Name: "anexo.pdf"}, nil
	}}

	recorder := serve(service, httptest.NewRequest(http.MethodGet, "/support/tickets/ticket-1/messages/m-1/attachment", nil))

	if recorder.Code != http.StatusOK || recorder.Body.String() != "%PDF" {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body)
	}
	if recorder.Header().Get("Cache-Control") != "private, no-store" || recorder.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("headers = %v", recorder.Header())
	}

	service.open = func(context.Context, string, string, string) (*support.AttachmentFile, error) {
		return nil, app.ErrAttachmentNotFound
	}
	if recorder := serve(service, httptest.NewRequest(http.MethodGet, "/support/tickets/ticket-1/messages/m-1/attachment", nil)); recorder.Code != http.StatusNotFound {
		t.Fatalf("missing: status = %d", recorder.Code)
	}
}

func TestListQueuePassesQueryAndShowsPeople(t *testing.T) {
	service := fakeService{queue: func(_ context.Context, adminID string, query support.QueueQuery, page uint, limit uint) (*support.TicketPage, error) {
		if adminID != userID || query.Status != "active" || query.Assigned != "none" || page != 1 || limit != 20 {
			t.Errorf("admin = %q, query = %+v, page = %d, limit = %d", adminID, query, page, limit)
		}
		entity := ticket().TicketEntity
		entity.OpenedBy, entity.OpenerName = "buyer-1", "Ana"
		return &support.TicketPage{Items: []*support.TicketEntity{&entity}, CurrentPage: 1, TotalPages: 1, TotalItems: 1}, nil
	}}

	recorder := serve(service, httptest.NewRequest(http.MethodGet, "/admin/support/tickets?status=active&assigned=none&limit=20", nil))

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"opened_by":{"id":"buyer-1","name":"Ana"}`) ||
		!strings.Contains(recorder.Body.String(), `"assigned_to":null`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	service.queue = func(context.Context, string, support.QueueQuery, uint, uint) (*support.TicketPage, error) {
		return nil, app.ErrInvalidInput
	}
	if recorder := serve(service, httptest.NewRequest(http.MethodGet, "/admin/support/tickets?assigned=x", nil)); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid: status = %d", recorder.Code)
	}
}

func TestAssignTicket(t *testing.T) {
	service := fakeService{assign: func(_ context.Context, adminID string, id string) (*support.TicketDetailEntity, error) {
		if adminID != userID || id != "ticket-1" {
			t.Errorf("admin = %q, id = %q", adminID, id)
		}
		assigned := ticket()
		assigned.AssignedTo = &adminID
		return assigned, nil
	}}

	recorder := serve(service, httptest.NewRequest(http.MethodPost, "/admin/support/tickets/ticket-1/assign", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"assigned_to_me":true`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	service.assign = func(context.Context, string, string) (*support.TicketDetailEntity, error) {
		return nil, app.ErrInvalidStatusTransition
	}
	if recorder := serve(service, httptest.NewRequest(http.MethodPost, "/admin/support/tickets/ticket-1/assign", nil)); recorder.Code != http.StatusConflict {
		t.Fatalf("resolved: status = %d", recorder.Code)
	}
}

func TestCloseTicket(t *testing.T) {
	service := fakeService{close: func(_ context.Context, _ string, id string, resolution string) (*support.TicketDetailEntity, error) {
		if id != "ticket-1" || resolution != "Reembolso aprovado." {
			t.Errorf("id = %q, resolution = %q", id, resolution)
		}
		closed := ticket()
		closed.Status, closed.Resolution = support.StatusResolved, &resolution
		return closed, nil
	}}

	recorder := serve(service, httptest.NewRequest(http.MethodPost, "/admin/support/tickets/ticket-1/close", strings.NewReader(`{"resolution":"Reembolso aprovado."}`)))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"resolution":"Reembolso aprovado."`) || !strings.Contains(recorder.Body.String(), `"can_reply":false`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	if recorder := serve(service, httptest.NewRequest(http.MethodPost, "/admin/support/tickets/ticket-1/close", strings.NewReader("{"))); recorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed: status = %d", recorder.Code)
	}

	service.close = func(context.Context, string, string, string) (*support.TicketDetailEntity, error) {
		return nil, &app.MissingFieldsError{Fields: []string{"resolution"}}
	}
	if recorder := serve(service, httptest.NewRequest(http.MethodPost, "/admin/support/tickets/ticket-1/close", strings.NewReader(`{}`))); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing: status = %d", recorder.Code)
	}
}
