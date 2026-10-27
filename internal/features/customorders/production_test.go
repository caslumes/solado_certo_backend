package customorders

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

var testPDF = []byte("%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n")

func testPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewGray(image.Rect(0, 0, 400, 400))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func approved() setup {
	st := newSetup().withRequest(StatusApproved)
	st.r.requests[testRequestID].QuotedPrice = ptr("189.90")
	return st
}

func checkoutInput() CheckoutInput {
	return CheckoutInput{AddressID: "00000000-0000-0000-0000-000000000701", ShippingOptionID: "00000000-0000-0000-0000-000000000702"}
}

func TestCheckout(t *testing.T) {
	st := approved()
	if output := st.r.requests[testRequestID].ToOutput(testBuyerID); !slices.Equal(output.Actions, []string{"cancel", "checkout"}) {
		t.Errorf("buyer actions before checkout = %v", output.Actions)
	}

	request, created, err := st.s.Checkout(context.Background(), testBuyerID, testRequestID, "key-1", checkoutInput())

	if err != nil || !created {
		t.Fatalf("Checkout: %v, created = %v", err, created)
	}
	item := st.planner.items[0]
	if item.RequestID != testRequestID || item.AdvertisementID != testAdID || item.Price != "189.90" || item.ShippingOptionID != checkoutInput().ShippingOptionID {
		t.Errorf("planned item = %+v, want the request's ad at the quoted price", item)
	}
	checkout := st.r.checkouts[0]
	if checkout.BuyerID != testBuyerID || checkout.IdempotencyKey != "key-1" || checkout.Plan == nil {
		t.Errorf("checkout = %+v", checkout)
	}
	if request.OrderID == nil || request.Status != StatusApproved {
		t.Errorf("request = %+v, want approved with its order", request)
	}
	if output := request.ToOutput(testBuyerID); len(output.Actions) != 0 {
		t.Errorf("buyer actions after checkout = %v, want none (pay or cancel the order)", output.Actions)
	}

	again, created, err := st.s.Checkout(context.Background(), testBuyerID, testRequestID, "key-1", checkoutInput())
	if err != nil || created || again.OrderID == nil || len(st.r.checkouts) != 1 {
		t.Errorf("repeated key: %v, created = %v, checkouts = %d, want the same request (RNF-012)", err, created, len(st.r.checkouts))
	}
	if _, _, err := st.s.Checkout(context.Background(), testBuyerID, testRequestID, "key-2", checkoutInput()); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("second checkout: error = %v, want ErrInvalidStatusTransition", err)
	}
}

func TestCheckoutRefuses(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*setup)
		userID string
		key    string
		input  CheckoutInput
		want   error
	}{
		{"no key", nil, testBuyerID, "", checkoutInput(), app.ErrInvalidInput},
		{"key too long", nil, testBuyerID, strings.Repeat("k", 121), checkoutInput(), app.ErrInvalidInput},
		{"no address", nil, testBuyerID, "k", CheckoutInput{ShippingOptionID: "x"}, app.ErrMissingRequiredFields},
		{"no shipping", nil, testBuyerID, "k", CheckoutInput{AddressID: "x"}, app.ErrMissingRequiredFields},
		{"another buyer", nil, testOtherID, "k", checkoutInput(), app.ErrCustomOrderNotFound},
		{"not accepted yet", func(st *setup) { st.r.requests[testRequestID].Status = StatusUnderReview }, testBuyerID, "k", checkoutInput(), app.ErrInvalidStatusTransition},
		{"plan fails", func(st *setup) { st.planner.err = app.ErrAddressNotFound }, testBuyerID, "k", checkoutInput(), app.ErrAddressNotFound},
		{"got an order meanwhile", func(st *setup) { st.r.checkoutErr = app.ErrInvalidStatusTransition }, testBuyerID, "k", checkoutInput(), app.ErrInvalidStatusTransition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := approved()
			if tt.setup != nil {
				tt.setup(&st)
			}

			_, _, err := st.s.Checkout(context.Background(), tt.userID, testRequestID, tt.key, tt.input)

			if !errors.Is(err, tt.want) && !(tt.want == app.ErrMissingRequiredFields && isMissingFields(err)) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
			if len(st.r.checkouts) != 0 {
				t.Errorf("checkouts = %+v, want none", st.r.checkouts)
			}
		})
	}
}

func isMissingFields(err error) bool {
	_, ok := errors.AsType[*app.MissingFieldsError](err)
	return ok
}

func inProduction() setup {
	st := approved()
	request := st.r.requests[testRequestID]
	request.Status, request.OrderID = StatusInProduction, ptr(testOrderID)
	return st
}

func TestReportRequest(t *testing.T) {
	st := inProduction()
	if output := st.r.requests[testRequestID].ToOutput(testSellerID); !slices.Equal(output.Actions, []string{"report"}) {
		t.Errorf("seller actions = %v", output.Actions)
	}

	request, err := st.s.ReportRequest(context.Background(), testSellerID, testRequestID, "  Use 2 horas no primeiro dia.  ", bytes.NewReader(testPDF))

	if err != nil {
		t.Fatalf("ReportRequest: %v", err)
	}
	change := st.r.changes[0]
	if change.Action != ActionReport || change.To != StatusCompleted || change.ReportKey == nil || !st.files.Has(storage.AreaPrivate, *change.ReportKey) ||
		!strings.HasPrefix(*change.ReportKey, reportsFolder+"/") || *change.AdaptationInstructions != "Use 2 horas no primeiro dia." {
		t.Errorf("change = %+v", change)
	}
	if n := st.r.notified[0]; n.UserID != testBuyerID || n.Type != notifications.TypeCustomOrderUpdated {
		t.Errorf("notification = %+v, want the buyer's", n)
	}
	output := request.ToOutput(testBuyerID)
	if output.Status != string(StatusCompleted) || output.FittingReport == nil || output.FittingReport.URL != "/documents/report-1" ||
		*output.AdaptationInstructions != "Use 2 horas no primeiro dia." {
		t.Errorf("output = %+v", output)
	}
}

func TestReportRequestRefuses(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(*setup)
		userID       string
		instructions string
		content      []byte
		want         error
	}{
		{"no instructions", nil, testSellerID, "  ", testPDF, app.ErrMissingRequiredFields},
		{"instructions too long", nil, testSellerID, strings.Repeat("a", maxInstructionsLength+1), testPDF, app.ErrInvalidInput},
		{"buyer", nil, testBuyerID, "Use aos poucos.", testPDF, app.ErrCustomOrderNotFound},
		{"not paid yet", func(st *setup) { st.r.requests[testRequestID].Status = StatusApproved }, testSellerID, "Use aos poucos.", testPDF, app.ErrInvalidStatusTransition},
		{"text file", nil, testSellerID, "Use aos poucos.", []byte("hello, this is not a report"), storage.ErrUnsupportedType},
		{"incomplete PDF", nil, testSellerID, "Use aos poucos.", []byte("%PDF-1.4\n1 0 obj"), app.ErrUnreadableDocument},
		{"lost to another change", func(st *setup) { st.r.changeErr = app.ErrInvalidStatusTransition }, testSellerID, "Use aos poucos.", testPDF, app.ErrInvalidStatusTransition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := inProduction()
			if tt.setup != nil {
				tt.setup(&st)
			}

			_, err := st.s.ReportRequest(context.Background(), tt.userID, testRequestID, tt.instructions, bytes.NewReader(tt.content))

			if !errors.Is(err, tt.want) && !(tt.want == app.ErrMissingRequiredFields && isMissingFields(err)) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
			if st.files.Count() != 0 {
				t.Errorf("%d files kept, want none", st.files.Count())
			}
		})
	}
}

func TestSendPhoto(t *testing.T) {
	st := newSetup().withRequest(StatusNeedsInfo)

	request, err := st.s.SendPhoto(context.Background(), testBuyerID, testRequestID, bytes.NewReader(testPNG(t)))

	if err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	first := st.r.photos[0].Key
	if !st.files.Has(storage.AreaPrivate, first) || !strings.HasPrefix(first, photosFolder+"/") || !strings.HasSuffix(first, ".png") {
		t.Errorf("photo key = %q", first)
	}
	if url := request.ToOutput(testBuyerID).FootPhotoURL; url == nil || *url != "/custom-orders/"+testRequestID+"/photo" {
		t.Errorf("buyer photo url = %v", url)
	}
	if url := request.ToOutput(testSellerID).FootPhotoURL; url == nil || *url != "/seller/custom-orders/"+testRequestID+"/photo" {
		t.Errorf("seller photo url = %v", url)
	}

	if _, err := st.s.SendPhoto(context.Background(), testBuyerID, testRequestID, bytes.NewReader(testPNG(t))); err != nil {
		t.Fatalf("second SendPhoto: %v", err)
	}
	if st.files.Has(storage.AreaPrivate, first) || st.files.Count() != 1 {
		t.Errorf("the replaced photo was kept: %d files", st.files.Count())
	}
}

func TestSendPhotoRefuses(t *testing.T) {
	tests := []struct {
		name    string
		status  Status
		setup   func(*setup)
		userID  string
		content func(*testing.T) []byte
		want    error
	}{
		{"seller decided", StatusApproved, nil, testBuyerID, testPNG, app.ErrInvalidStatusTransition},
		{"another buyer", StatusRequested, nil, testOtherID, testPNG, app.ErrCustomOrderNotFound},
		{"no consent", StatusRequested, func(st *setup) { st.consents.granted = map[string]bool{} }, testBuyerID, testPNG, app.ErrConsentRequired},
		{"PDF", StatusRequested, nil, testBuyerID, func(*testing.T) []byte { return testPDF }, storage.ErrUnsupportedType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newSetup().withRequest(tt.status)
			if tt.setup != nil {
				tt.setup(&st)
			}

			_, err := st.s.SendPhoto(context.Background(), tt.userID, testRequestID, bytes.NewReader(tt.content(t)))

			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
			if st.files.Count() != 0 {
				t.Errorf("%d files kept, want none", st.files.Count())
			}
		})
	}
}

func TestOpenPhoto(t *testing.T) {
	st := newSetup().withRequest(StatusRequested)
	ctx := context.Background()
	if _, err := st.s.OpenPhoto(ctx, testBuyerID, testRequestID); !errors.Is(err, app.ErrFootPhotoNotFound) {
		t.Errorf("no photo: error = %v, want ErrFootPhotoNotFound", err)
	}
	content := testPNG(t)
	if _, err := st.s.SendPhoto(ctx, testBuyerID, testRequestID, bytes.NewReader(content)); err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}

	for _, open := range []struct {
		userID string
		call   func(context.Context, string, string) (*PhotoFile, error)
	}{{testBuyerID, st.s.OpenPhoto}, {testSellerID, st.s.OpenSellerPhoto}} {
		file, err := open.call(ctx, open.userID, testRequestID)
		if err != nil {
			t.Fatalf("%s: open: %v", open.userID, err)
		}
		read, _ := io.ReadAll(file)
		file.Close()
		if !bytes.Equal(read, content) || file.ContentType != "image/png" || file.Name != "foot-photo.png" {
			t.Errorf("%s: file = %s %s, %d bytes", open.userID, file.ContentType, file.Name, len(read))
		}
	}
	if !slices.Equal(st.r.photoViews, []string{testSellerID}) {
		t.Errorf("audited views = %v, want only the seller's", st.r.photoViews)
	}
	if _, err := st.s.OpenSellerPhoto(ctx, testOtherID, testRequestID); !errors.Is(err, app.ErrCustomOrderNotFound) {
		t.Errorf("another seller: error = %v, want ErrCustomOrderNotFound", err)
	}
}
