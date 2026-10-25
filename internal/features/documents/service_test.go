package documents

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

const (
	testBuyerID    = "buyer-1"
	testSellerID   = "seller-1"
	testAdminID    = "admin-1"
	testOrderID    = "00000000-0000-0000-0000-000000000801"
	testItemID     = "00000000-0000-0000-0000-000000000811"
	testDocumentID = "00000000-0000-0000-0000-000000000901"
	unknownID      = "00000000-0000-0000-0000-000000000999"
)

var testPDF = []byte("%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n")

func testPNG(t *testing.T, side int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewGray(image.Rect(0, 0, side, side))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// hugePNG is a small file whose header claims more pixels than maxImagePixels.
func hugePNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewGray(image.Rect(0, 0, 7000, 7000))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

type fakeRepository struct {
	item       *PrescriptionItem
	getItemErr error
	created    []NewDocument
	createErr  error
	document   *StoredDocument
	admins     map[string]bool
	views      []string
	viewErr    error
	rejections []Rejection
	rejectErr  error
}

func (f *fakeRepository) GetPrescriptionItem(_ context.Context, _ string, itemID string) (*PrescriptionItem, error) {
	if f.getItemErr != nil {
		return nil, f.getItemErr
	}
	item := *f.item
	if itemID != item.ID {
		item.ID = ""
	}
	return &item, nil
}

func (f *fakeRepository) CreateDocument(_ context.Context, document NewDocument) (*DocumentEntity, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, document)
	return &DocumentEntity{ID: testDocumentID, OrderID: document.OrderID, OrderItemID: &document.OrderItemID, Type: document.Type, Status: document.Status}, nil
}

func (f *fakeRepository) GetDocument(_ context.Context, id string) (*StoredDocument, error) {
	if f.document == nil || id != f.document.ID {
		return nil, app.ErrDocumentNotFound
	}
	document := *f.document
	return &document, nil
}

func (f *fakeRepository) IsAdmin(_ context.Context, userID string) (bool, error) {
	return f.admins[userID], nil
}

func (f *fakeRepository) RecordView(_ context.Context, _ string, userID string) error {
	if f.viewErr != nil {
		return f.viewErr
	}
	f.views = append(f.views, userID)
	return nil
}

func (f *fakeRepository) RejectPrescription(_ context.Context, rejection Rejection) (*DocumentEntity, error) {
	if f.rejectErr != nil {
		return nil, f.rejectErr
	}
	f.rejections = append(f.rejections, rejection)
	return &DocumentEntity{ID: rejection.DocumentID, Status: orders.DocumentStatusRejected, RejectionReason: &rejection.Reason}, nil
}

type fakeConsents struct {
	granted map[string]bool
}

func (f *fakeConsents) RequireConsent(_ context.Context, userID string, purpose me.ConsentPurpose) error {
	if purpose != me.ConsentPurposePrescription || !f.granted[userID] {
		return app.ErrConsentRequired
	}
	return nil
}

type setup struct {
	r        *fakeRepository
	files    *storage.MemoryStorage
	consents *fakeConsents
	s        *service
}

// newSetup has an unpaid order of testBuyerID whose item testItemID needs a prescription, and a
// prescription consent from the buyer.
func newSetup() *setup {
	r := &fakeRepository{
		item: &PrescriptionItem{
			ID: testItemID, Title: "Órtese corretora", OrderID: testOrderID, OrderNumber: "SC-1",
			OrderStatus: orders.OrderStatusAwaitingPayment, BuyerID: testBuyerID, SellerID: testSellerID, PrescriptionRequired: true,
		},
		admins: map[string]bool{testAdminID: true},
	}
	files := storage.NewMemoryStorage()
	consents := &fakeConsents{granted: map[string]bool{testBuyerID: true}}
	return &setup{r: r, files: files, consents: consents, s: NewService(r, files, consents)}
}

func (s *setup) upload(content []byte) (*DocumentEntity, error) {
	return s.s.UploadPrescription(context.Background(), testBuyerID, testOrderID, testItemID, bytes.NewReader(content))
}

func TestUploadPrescription(t *testing.T) {
	for name, content := range map[string][]byte{"pdf": testPDF, "png": testPNG(t, minImageSide)} {
		s := newSetup()

		document, err := s.upload(content)

		if err != nil {
			t.Fatalf("%s: UploadPrescription: %v", name, err)
		}
		if document.Status != orders.DocumentStatusApproved || document.Type != DocumentTypePrescription {
			t.Errorf("%s: document = %+v", name, document)
		}
		got := s.r.created[0]
		if got.OrderID != testOrderID || got.OrderItemID != testItemID || got.UploadedBy != testBuyerID || got.Notification != nil {
			t.Errorf("%s: created = %+v", name, got)
		}
		if !strings.HasPrefix(got.Key, prescriptionsFolder+"/") || !s.files.Has(storage.AreaPrivate, got.Key) {
			t.Errorf("%s: key %q not saved in the private area", name, got.Key)
		}
	}
}

func TestUploadPrescriptionAfterRejectionNotifiesTheSeller(t *testing.T) {
	s := newSetup()
	s.r.item.OrderStatus = orders.OrderStatusProcessing
	s.r.item.Latest = &orders.PrescriptionEntity{ID: "doc-0", Status: orders.DocumentStatusRejected}

	if _, err := s.upload(testPDF); err != nil {
		t.Fatalf("UploadPrescription: %v", err)
	}
	notification := s.r.created[0].Notification
	if notification == nil || notification.UserID != testSellerID || notification.Type != notifications.TypePrescriptionSent ||
		notification.Data["order_id"] != testOrderID {
		t.Errorf("notification = %+v", notification)
	}
}

func TestUploadPrescriptionRejectsTheFile(t *testing.T) {
	tests := map[string]struct {
		content []byte
		want    error
	}{
		"empty":           {nil, storage.ErrUnsupportedType},
		"text":            {[]byte("receita: palmilha"), storage.ErrUnsupportedType},
		"too large":       {append(bytes.Clone(testPDF), make([]byte, app.MaxUploadSize)...), storage.ErrTooLarge},
		"incomplete pdf":  {testPDF[:30], app.ErrUnreadableDocument},
		"small image":     {testPNG(t, minImageSide-1), app.ErrUnreadableDocument},
		"broken png":      {testPNG(t, minImageSide)[:40], app.ErrUnreadableDocument},
		"huge image":      {hugePNG(t), app.ErrUnreadableDocument},
		"pdf eof too far": {append(bytes.Clone(testPDF), bytes.Repeat([]byte(" "), pdfTrailerWindow)...), app.ErrUnreadableDocument},
	}
	for name, test := range tests {
		s := newSetup()
		if _, err := s.upload(test.content); !errors.Is(err, test.want) {
			t.Errorf("%s: error = %v, want %v", name, err, test.want)
		}
		if len(s.r.created) != 0 {
			t.Errorf("%s: created = %+v", name, s.r.created)
		}
	}
}

func TestUploadPrescriptionRefuses(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*setup)
		buyer   string
		order   string
		item    string
		wantErr error
	}{
		{"bad order id", nil, testBuyerID, "x", testItemID, app.ErrOrderNotFound},
		{"bad item id", nil, testBuyerID, testOrderID, "x", app.ErrOrderItemNotFound},
		{"unknown order", func(s *setup) { s.r.getItemErr = app.ErrOrderNotFound }, testBuyerID, testOrderID, testItemID, app.ErrOrderNotFound},
		{"another buyer", nil, "buyer-2", testOrderID, testItemID, app.ErrOrderNotFound},
		{"unknown item", nil, testBuyerID, testOrderID, unknownID, app.ErrOrderItemNotFound},
		{"not needed", func(s *setup) { s.r.item.PrescriptionRequired = false }, testBuyerID, testOrderID, testItemID, app.ErrPrescriptionNotNeeded},
		{"paid, already approved", func(s *setup) {
			s.r.item.OrderStatus = orders.OrderStatusPaid
			s.r.item.Latest = &orders.PrescriptionEntity{Status: orders.DocumentStatusApproved}
		}, testBuyerID, testOrderID, testItemID, app.ErrInvalidStatusTransition},
		{"shipped", func(s *setup) { s.r.item.OrderStatus = orders.OrderStatusShipped }, testBuyerID, testOrderID, testItemID, app.ErrInvalidStatusTransition},
		{"no consent", func(s *setup) { s.consents.granted = nil }, testBuyerID, testOrderID, testItemID, app.ErrConsentRequired},
		{"repository failure", func(s *setup) { s.r.getItemErr = errors.New("db down") }, testBuyerID, testOrderID, testItemID, nil},
	}
	for _, test := range tests {
		s := newSetup()
		if test.setup != nil {
			test.setup(s)
		}
		_, err := s.s.UploadPrescription(context.Background(), test.buyer, test.order, test.item, bytes.NewReader(testPDF))
		if test.wantErr == nil && err == nil || test.wantErr != nil && !errors.Is(err, test.wantErr) {
			t.Errorf("%s: error = %v, want %v", test.name, err, test.wantErr)
		}
		if len(s.r.created) != 0 {
			t.Errorf("%s: created = %+v", test.name, s.r.created)
		}
	}
}

func TestUploadPrescriptionDeletesTheFileWhenNotStored(t *testing.T) {
	s := newSetup()
	s.r.createErr = errors.New("db down")

	if _, err := s.upload(testPDF); !errors.Is(err, s.r.createErr) {
		t.Fatalf("error = %v, want %v", err, s.r.createErr)
	}
	if s.files.Has(storage.AreaPrivate, prescriptionsFolder+"/00000000000000000000000000000001.pdf") {
		t.Error("the file was kept")
	}
}

// storedSetup has a prescription of testBuyerID saved for a paid order of testSellerID.
func storedSetup(t *testing.T) *setup {
	s := newSetup()
	key, err := s.files.Save(context.Background(), storage.AreaPrivate, prescriptionsFolder, ".pdf", bytes.NewReader(testPDF))
	if err != nil {
		t.Fatal(err)
	}
	itemID := testItemID
	s.r.document = &StoredDocument{
		DocumentEntity: DocumentEntity{ID: testDocumentID, OrderID: testOrderID, OrderItemID: &itemID, Type: DocumentTypePrescription, Status: orders.DocumentStatusApproved},
		Key:            key,
		UploadedBy:     testBuyerID,
		OrderNumber:    "SC-1",
		ItemTitle:      "Órtese corretora",
		BuyerID:        testBuyerID,
		SellerID:       testSellerID,
		OrderPaid:      true,
	}
	return s
}

func TestOpenDocument(t *testing.T) {
	for _, user := range []string{testBuyerID, testSellerID, testAdminID} {
		s := storedSetup(t)

		file, err := s.s.OpenDocument(context.Background(), user, testDocumentID)

		if err != nil {
			t.Fatalf("%s: OpenDocument: %v", user, err)
		}
		content, _ := io.ReadAll(file)
		file.Close()
		if !bytes.Equal(content, testPDF) || file.ContentType != "application/pdf" || file.Name != "prescription-SC-1.pdf" {
			t.Errorf("%s: file = %q, %s, %s", user, content, file.ContentType, file.Name)
		}
		wantViews := 1
		if user == testBuyerID {
			wantViews = 0
		}
		if len(s.r.views) != wantViews {
			t.Errorf("%s: views = %v, want %d", user, s.r.views, wantViews)
		}
	}
}

func TestOpenDocumentRefuses(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*setup)
		user  string
		id    string
	}{
		{"bad id", nil, testBuyerID, "x"},
		{"unknown document", nil, testBuyerID, unknownID},
		{"another buyer", nil, "buyer-2", testDocumentID},
		{"another seller", nil, "seller-2", testDocumentID},
		{"seller of an unpaid order", func(s *setup) { s.r.document.OrderPaid = false }, testSellerID, testDocumentID},
		{"file missing", func(s *setup) { s.files.Delete(context.Background(), storage.AreaPrivate, s.r.document.Key) }, testBuyerID, testDocumentID},
	}
	for _, test := range tests {
		s := storedSetup(t)
		if test.setup != nil {
			test.setup(s)
		}
		if _, err := s.s.OpenDocument(context.Background(), test.user, test.id); !errors.Is(err, app.ErrDocumentNotFound) {
			t.Errorf("%s: error = %v, want ErrDocumentNotFound", test.name, err)
		}
	}
}

func TestOpenDocumentFailsWithoutTheAuditRow(t *testing.T) {
	s := storedSetup(t)
	s.r.viewErr = errors.New("db down")

	if _, err := s.s.OpenDocument(context.Background(), testSellerID, testDocumentID); !errors.Is(err, s.r.viewErr) {
		t.Errorf("error = %v, want %v", err, s.r.viewErr)
	}
}

func TestRejectPrescription(t *testing.T) {
	s := storedSetup(t)

	document, err := s.s.RejectPrescription(context.Background(), testSellerID, testOrderID, testDocumentID, RejectionInput{Reason: "  Receita sem assinatura.  "})

	if err != nil {
		t.Fatalf("RejectPrescription: %v", err)
	}
	if document.Status != orders.DocumentStatusRejected || *document.RejectionReason != "Receita sem assinatura." {
		t.Errorf("document = %+v", document)
	}
	got := s.r.rejections[0]
	if got.DocumentID != testDocumentID || got.OrderItemID != testItemID || got.SellerID != testSellerID || got.Reason != "Receita sem assinatura." {
		t.Errorf("rejection = %+v", got)
	}
	if got.Notification.UserID != testBuyerID || got.Notification.Type != notifications.TypePrescriptionRejected ||
		!strings.Contains(got.Notification.Body, "Receita sem assinatura. Envie") {
		t.Errorf("notification = %+v", got.Notification)
	}
}

func TestRejectPrescriptionRefuses(t *testing.T) {
	long := strings.Repeat("a", maxReasonLength+1)
	tests := []struct {
		name    string
		setup   func(*setup)
		seller  string
		order   string
		reason  string
		wantErr error
	}{
		{"no reason", nil, testSellerID, testOrderID, "  ", app.ErrMissingRequiredFields},
		{"long reason", nil, testSellerID, testOrderID, long, app.ErrInvalidInput},
		{"another seller", nil, "seller-2", testOrderID, "x", app.ErrDocumentNotFound},
		{"another order", nil, testSellerID, unknownID, "x", app.ErrDocumentNotFound},
		{"unpaid order", func(s *setup) { s.r.document.OrderPaid = false }, testSellerID, testOrderID, "x", app.ErrDocumentNotFound},
		{"not a prescription", func(s *setup) { s.r.document.Type = DocumentTypeTechnicalReport }, testSellerID, testOrderID, "x", app.ErrDocumentNotFound},
		{"not rejectable", func(s *setup) { s.r.rejectErr = app.ErrInvalidStatusTransition }, testSellerID, testOrderID, "x", app.ErrInvalidStatusTransition},
	}
	for _, test := range tests {
		s := storedSetup(t)
		if test.setup != nil {
			test.setup(s)
		}
		_, err := s.s.RejectPrescription(context.Background(), test.seller, test.order, testDocumentID, RejectionInput{Reason: test.reason})
		if !errors.Is(err, test.wantErr) {
			t.Errorf("%s: error = %v, want %v", test.name, err, test.wantErr)
		}
		if len(s.r.rejections) != 0 {
			t.Errorf("%s: rejections = %+v", test.name, s.r.rejections)
		}
	}
}
