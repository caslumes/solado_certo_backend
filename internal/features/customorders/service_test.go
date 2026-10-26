package customorders

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
)

const (
	testBuyerID   = "buyer-1"
	testSellerID  = "seller-1"
	testOtherID   = "other-1"
	testAdID      = "00000000-0000-0000-0000-000000000601"
	testRequestID = "00000000-0000-0000-0000-000000000901"
)

type fakeRepository struct {
	ad       *CustomizableAd
	requests map[string]*RequestDetailEntity
	created  []NewRequest
	changes  []StatusChange
	notified []notifications.NewNotification
	filter   Filter
	total    uint
	// changeErr is returned by ChangeStatus, as when another change got there first.
	changeErr error
}

func (f *fakeRepository) GetAdvertisement(_ context.Context, id string) (*CustomizableAd, error) {
	if f.ad == nil || f.ad.ID != id {
		return nil, app.ErrAdvertisementNotFound
	}
	return f.ad, nil
}

func (f *fakeRepository) CreateRequest(_ context.Context, request NewRequest) (string, error) {
	f.created = append(f.created, request)
	f.notified = append(f.notified, request.Notification(testRequestID))
	f.requests[testRequestID] = &RequestDetailEntity{
		RequestEntity: RequestEntity{
			ID: testRequestID, BuyerID: request.BuyerID, SellerID: f.ad.SellerID, AdvertisementID: request.AdvertisementID,
			AdvertisementTitle: f.ad.Title, Status: StatusRequested, Measurements: request.Measurements, TechnicalNotes: request.TechnicalNotes,
		},
		History: []HistoryEntity{{Status: StatusRequested, ChangedBy: &request.BuyerID}},
	}
	return testRequestID, nil
}

func (f *fakeRepository) GetRequest(_ context.Context, id string) (*RequestDetailEntity, error) {
	request, ok := f.requests[id]
	if !ok {
		return nil, app.ErrCustomOrderNotFound
	}
	copied := *request
	copied.History = slices.Clone(request.History)
	return &copied, nil
}

func (f *fakeRepository) CountRequests(_ context.Context, filter Filter) (uint, error) {
	f.filter = filter
	return f.total, nil
}

func (f *fakeRepository) GetRequests(context.Context, Filter, uint, uint) ([]*RequestEntity, error) {
	var items []*RequestEntity
	for _, request := range f.requests {
		items = append(items, &request.RequestEntity)
	}
	return items, nil
}

func (f *fakeRepository) ChangeStatus(_ context.Context, change StatusChange) error {
	if f.changeErr != nil {
		return f.changeErr
	}
	request := f.requests[change.RequestID]
	if request.Status != change.From {
		return app.ErrInvalidStatusTransition
	}
	f.changes = append(f.changes, change)
	f.notified = append(f.notified, change.Notification)
	from := request.Status
	request.Status = change.To
	if change.QuotedPrice != nil {
		request.QuotedPrice = change.QuotedPrice
	}
	if change.RejectionReason != nil {
		request.RejectionReason = change.RejectionReason
	}
	if change.Measurements != nil {
		request.Measurements = *change.Measurements
	}
	if change.TechnicalNotes != nil {
		request.TechnicalNotes = change.TechnicalNotes
	}
	request.History = append(request.History, HistoryEntity{PreviousStatus: &from, Status: change.To, ChangedBy: &change.ChangedBy, Reason: change.Reason, CreatedAt: time.Now()})
	return nil
}

type fakeConsents struct {
	granted map[string]bool
}

func (f *fakeConsents) RequireConsent(_ context.Context, userID string, purpose me.ConsentPurpose) error {
	if purpose != me.ConsentPurposeCustomOrder || !f.granted[userID] {
		return app.ErrConsentRequired
	}
	return nil
}

type setup struct {
	r        *fakeRepository
	consents *fakeConsents
	s        *service
}

func newSetup() setup {
	r := &fakeRepository{
		ad:       &CustomizableAd{ID: testAdID, SellerID: testSellerID, Title: "Palmilha sob medida", Published: true, CustomMadeAllowed: true},
		requests: map[string]*RequestDetailEntity{},
	}
	consents := &fakeConsents{granted: map[string]bool{testBuyerID: true}}
	return setup{r: r, consents: consents, s: NewService(r, consents)}
}

// withRequest has a request of testBuyerID in status.
func (st setup) withRequest(status Status) setup {
	st.r.requests[testRequestID] = &RequestDetailEntity{RequestEntity: RequestEntity{
		ID: testRequestID, BuyerID: testBuyerID, SellerID: testSellerID, AdvertisementID: testAdID,
		AdvertisementTitle: "Palmilha sob medida", Status: status, Measurements: validMeasurements(),
	}}
	return st
}

func ptr[T any](v T) *T {
	return &v
}

func validMeasurements() Measurements {
	return Measurements{Left: &Foot{LengthCm: 25.04, WidthCm: 9.66}, Right: &Foot{LengthCm: 25.3, WidthCm: 9.8}, ArchType: ArchTypeFlat, ShoeSize: 39}
}

func validInput() RequestInput {
	m := validMeasurements()
	return RequestInput{AdvertisementID: testAdID, Measurements: &m, TechnicalNotes: ptr("  Esporão no calcanhar esquerdo.  ")}
}

func TestCreateRequest(t *testing.T) {
	st := newSetup()

	request, err := st.s.CreateRequest(context.Background(), testBuyerID, validInput())

	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if request.ID != testRequestID || request.Status != StatusRequested {
		t.Errorf("request = %+v", request)
	}
	got := st.r.created[0]
	if got.BuyerID != testBuyerID || got.AdvertisementID != testAdID || *got.TechnicalNotes != "Esporão no calcanhar esquerdo." {
		t.Errorf("created = %+v", got)
	}
	if got.Measurements.Left.LengthCm != 25 || got.Measurements.Left.WidthCm != 9.7 || got.Measurements.Right.LengthCm != 25.3 {
		t.Errorf("measurements not rounded to millimetres: left %+v, right %+v", *got.Measurements.Left, *got.Measurements.Right)
	}
	notification := st.r.notified[0]
	if notification.UserID != testSellerID || notification.Type != notifications.TypeCustomOrderRequested || notification.Data["custom_order_id"] != testRequestID {
		t.Errorf("notification = %+v", notification)
	}
	if output := request.ToOutput(testBuyerID); !slices.Equal(output.Actions, []string{"cancel"}) {
		t.Errorf("buyer actions = %v", output.Actions)
	}
}

func TestCreateRequestValidatesTheInput(t *testing.T) {
	missingSide := validMeasurements()
	missingSide.Right = nil
	tooLong := validMeasurements()
	tooLong.Left = &Foot{LengthCm: 40, WidthCm: 9}
	tooNarrow := validMeasurements()
	tooNarrow.Right = &Foot{LengthCm: 25, WidthCm: 2}
	badArch := validMeasurements()
	badArch.ArchType = "curvo"
	badSize := validMeasurements()
	badSize.ShoeSize = 60
	longNotes := strings.Repeat("a", maxNotesLength+1)

	for name, test := range map[string]struct {
		input RequestInput
		want  error
	}{
		"no ad":               {RequestInput{Measurements: ptr(validMeasurements())}, app.ErrMissingRequiredFields},
		"no measurements":     {RequestInput{AdvertisementID: testAdID}, app.ErrMissingRequiredFields},
		"one side missing":    {RequestInput{AdvertisementID: testAdID, Measurements: &missingSide}, app.ErrMissingRequiredFields},
		"length out of range": {RequestInput{AdvertisementID: testAdID, Measurements: &tooLong}, app.ErrInvalidInput},
		"width out of range":  {RequestInput{AdvertisementID: testAdID, Measurements: &tooNarrow}, app.ErrInvalidInput},
		"unknown arch type":   {RequestInput{AdvertisementID: testAdID, Measurements: &badArch}, app.ErrInvalidInput},
		"shoe size":           {RequestInput{AdvertisementID: testAdID, Measurements: &badSize}, app.ErrInvalidInput},
		"long notes":          {RequestInput{AdvertisementID: testAdID, Measurements: ptr(validMeasurements()), TechnicalNotes: &longNotes}, app.ErrInvalidInput},
		"ad id not a uuid":    {RequestInput{AdvertisementID: "abc", Measurements: ptr(validMeasurements())}, app.ErrAdvertisementNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			st := newSetup()
			_, err := st.s.CreateRequest(context.Background(), testBuyerID, test.input)
			if !errors.Is(err, test.want) {
				t.Errorf("err = %v, want %v", err, test.want)
			}
			if len(st.r.created) != 0 {
				t.Error("request created")
			}
		})
	}
}

func TestCreateRequestChecksTheAdAndConsent(t *testing.T) {
	for name, test := range map[string]struct {
		change func(st setup)
		buyer  string
		want   error
	}{
		"unknown ad":         {func(st setup) { st.r.ad.ID = testRequestID }, testBuyerID, app.ErrAdvertisementNotFound},
		"ad not published":   {func(st setup) { st.r.ad.Published = false }, testBuyerID, app.ErrAdvertisementNotFound},
		"ad not custom made": {func(st setup) { st.r.ad.CustomMadeAllowed = false }, testBuyerID, app.ErrCustomMadeNotAllowed},
		"own ad":             {func(st setup) { st.consents.granted[testSellerID] = true }, testSellerID, app.ErrOwnAdvertisement},
		"no consent":         {func(st setup) { st.consents.granted[testBuyerID] = false }, testBuyerID, app.ErrConsentRequired},
	} {
		t.Run(name, func(t *testing.T) {
			st := newSetup()
			test.change(st)
			_, err := st.s.CreateRequest(context.Background(), test.buyer, validInput())
			if !errors.Is(err, test.want) {
				t.Errorf("err = %v, want %v", err, test.want)
			}
			if len(st.r.created) != 0 {
				t.Error("request created")
			}
		})
	}
}

// TestRequestGoesThroughNeedsInfoToAccepted walks UC-008's alternative flow: the seller asks for more
// information, the buyer complements, and the seller accepts with a price.
func TestRequestGoesThroughNeedsInfoToAccepted(t *testing.T) {
	st := newSetup().withRequest(StatusRequested)
	ctx := context.Background()

	request, err := st.s.ReviewRequest(ctx, testSellerID, testRequestID)
	if err != nil || request.Status != StatusUnderReview {
		t.Fatalf("ReviewRequest: %v, %+v", err, request)
	}
	request, err = st.s.RequestInfo(ctx, testSellerID, testRequestID, ReasonInput{Reason: " Qual a altura do arco? "})
	if err != nil || request.Status != StatusNeedsInfo {
		t.Fatalf("RequestInfo: %v, %+v", err, request)
	}
	if output := request.ToOutput(testBuyerID); !slices.Equal(output.Actions, []string{"complement", "cancel"}) {
		t.Errorf("buyer actions = %v", output.Actions)
	}
	if output := request.ToOutput(testSellerID); !slices.Equal(output.Actions, []string{"reject"}) {
		t.Errorf("seller actions = %v", output.Actions)
	}

	newMeasurements := validMeasurements()
	newMeasurements.ArchType = ArchTypeHigh
	request, err = st.s.ComplementRequest(ctx, testBuyerID, testRequestID, ComplementInput{Message: "Arco alto, 3 cm.", Measurements: &newMeasurements})
	if err != nil || request.Status != StatusUnderReview || request.Measurements.ArchType != ArchTypeHigh {
		t.Fatalf("ComplementRequest: %v, %+v", err, request)
	}
	request, err = st.s.AcceptRequest(ctx, testSellerID, testRequestID, AcceptInput{QuotedPrice: "189.9"})
	if err != nil || request.Status != StatusApproved || *request.QuotedPrice != "189.90" {
		t.Fatalf("AcceptRequest: %v, %+v", err, request)
	}

	var reasons []string
	for _, h := range request.ToOutput(testBuyerID).History {
		reason := ""
		if h.Reason != nil {
			reason = *h.Reason
		}
		reasons = append(reasons, h.ChangedBy+":"+h.Status+":"+reason)
	}
	want := []string{"seller:under_review:", "seller:needs_info:Qual a altura do arco?", "buyer:under_review:Arco alto, 3 cm.", "seller:approved:"}
	if !slices.Equal(reasons, want) {
		t.Errorf("history = %v, want %v", reasons, want)
	}

	var told []string
	for _, n := range st.r.notified {
		told = append(told, n.UserID+":"+n.Title)
	}
	wantTold := []string{
		testBuyerID + ":Solicitação em análise",
		testBuyerID + ":O vendedor precisa de mais informações",
		testSellerID + ":Solicitação complementada",
		testBuyerID + ":Solicitação aceita",
	}
	if !slices.Equal(told, wantTold) {
		t.Errorf("notifications = %v, want %v", told, wantTold)
	}
	if !strings.Contains(st.r.notified[3].Body, "R$ 189,90") {
		t.Errorf("accepted body = %q", st.r.notified[3].Body)
	}
}

func TestRejectRequestKeepsTheReason(t *testing.T) {
	st := newSetup().withRequest(StatusUnderReview)

	request, err := st.s.RejectRequest(context.Background(), testSellerID, testRequestID, ReasonInput{Reason: "Não trabalhamos com esse número."})

	if err != nil {
		t.Fatalf("RejectRequest: %v", err)
	}
	if request.Status != StatusRejected || *request.RejectionReason != "Não trabalhamos com esse número." {
		t.Errorf("request = %+v", request)
	}
	if len(request.ToOutput(testBuyerID).Actions) != 0 || len(request.ToOutput(testSellerID).Actions) != 0 {
		t.Error("a rejected request still has actions")
	}
	if n := st.r.notified[0]; n.UserID != testBuyerID || !strings.Contains(n.Body, "Não trabalhamos com esse número.") {
		t.Errorf("notification = %+v", n)
	}
}

func TestStatusMachine(t *testing.T) {
	allowed := map[Action][]Status{
		ActionReview:      {StatusRequested},
		ActionAccept:      {StatusRequested, StatusUnderReview},
		ActionReject:      {StatusRequested, StatusUnderReview, StatusNeedsInfo},
		ActionRequestInfo: {StatusRequested, StatusUnderReview},
		ActionComplement:  {StatusNeedsInfo},
		ActionCancel:      {StatusRequested, StatusUnderReview, StatusNeedsInfo, StatusApproved},
	}
	for action, from := range allowed {
		for _, status := range statuses {
			if got := status.Allows(action); got != slices.Contains(from, status) {
				t.Errorf("%s allows %s = %v", status, action, got)
			}
		}
	}
}

func TestActionsRefusedInTheWrongStatus(t *testing.T) {
	ctx := context.Background()
	for name, test := range map[string]struct {
		status Status
		act    func(s *service) error
	}{
		"review twice": {StatusUnderReview, func(s *service) error { _, err := s.ReviewRequest(ctx, testSellerID, testRequestID); return err }},
		"accept while waiting for the buyer": {StatusNeedsInfo, func(s *service) error {
			_, err := s.AcceptRequest(ctx, testSellerID, testRequestID, AcceptInput{QuotedPrice: "10"})
			return err
		}},
		"complement without a question": {StatusUnderReview, func(s *service) error {
			_, err := s.ComplementRequest(ctx, testBuyerID, testRequestID, ComplementInput{Message: "Oi"})
			return err
		}},
		"cancel a rejected request": {StatusRejected, func(s *service) error {
			_, err := s.CancelRequest(ctx, testBuyerID, testRequestID, CancelInput{})
			return err
		}},
		"reject an approved request": {StatusApproved, func(s *service) error {
			_, err := s.RejectRequest(ctx, testSellerID, testRequestID, ReasonInput{Reason: "Desisti"})
			return err
		}},
	} {
		t.Run(name, func(t *testing.T) {
			st := newSetup().withRequest(test.status)
			if err := test.act(st.s); !errors.Is(err, app.ErrInvalidStatusTransition) {
				t.Errorf("err = %v, want ErrInvalidStatusTransition", err)
			}
			if len(st.r.changes) != 0 {
				t.Error("status changed")
			}
		})
	}
}

func TestChangeLostToAConcurrentOne(t *testing.T) {
	st := newSetup().withRequest(StatusRequested)
	st.r.changeErr = app.ErrInvalidStatusTransition

	_, err := st.s.ReviewRequest(context.Background(), testSellerID, testRequestID)

	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("err = %v", err)
	}
}

func TestOnlyThePartiesSeeAndChangeARequest(t *testing.T) {
	ctx := context.Background()
	st := newSetup().withRequest(StatusRequested)

	if _, err := st.s.GetRequest(ctx, testOtherID, testRequestID); !errors.Is(err, app.ErrCustomOrderNotFound) {
		t.Errorf("other buyer GetRequest err = %v", err)
	}
	if _, err := st.s.GetRequest(ctx, testSellerID, testRequestID); !errors.Is(err, app.ErrCustomOrderNotFound) {
		t.Errorf("seller through buyer route err = %v", err)
	}
	if _, err := st.s.GetSellerRequest(ctx, testOtherID, testRequestID); !errors.Is(err, app.ErrCustomOrderNotFound) {
		t.Errorf("other seller GetSellerRequest err = %v", err)
	}
	if _, err := st.s.AcceptRequest(ctx, testOtherID, testRequestID, AcceptInput{QuotedPrice: "10"}); !errors.Is(err, app.ErrCustomOrderNotFound) {
		t.Errorf("other seller AcceptRequest err = %v", err)
	}
	if _, err := st.s.CancelRequest(ctx, testOtherID, testRequestID, CancelInput{}); !errors.Is(err, app.ErrCustomOrderNotFound) {
		t.Errorf("other buyer CancelRequest err = %v", err)
	}
	if _, err := st.s.GetRequest(ctx, testBuyerID, "abc"); !errors.Is(err, app.ErrCustomOrderNotFound) {
		t.Errorf("non-UUID id err = %v", err)
	}
	if len(st.r.changes) != 0 {
		t.Error("status changed")
	}
}

func TestDecisionsNeedTheirFields(t *testing.T) {
	ctx := context.Background()
	for name, test := range map[string]struct {
		act  func(s *service) error
		want error
	}{
		"accept without price": {func(s *service) error {
			_, err := s.AcceptRequest(ctx, testSellerID, testRequestID, AcceptInput{})
			return err
		}, app.ErrMissingRequiredFields},
		"accept with zero price": {func(s *service) error {
			_, err := s.AcceptRequest(ctx, testSellerID, testRequestID, AcceptInput{QuotedPrice: "0"})
			return err
		}, app.ErrInvalidInput},
		"accept with bad price": {func(s *service) error {
			_, err := s.AcceptRequest(ctx, testSellerID, testRequestID, AcceptInput{QuotedPrice: "12,50"})
			return err
		}, app.ErrInvalidInput},
		"accept above numeric(10,2)": {func(s *service) error {
			_, err := s.AcceptRequest(ctx, testSellerID, testRequestID, AcceptInput{QuotedPrice: "100000000.00"})
			return err
		}, app.ErrInvalidInput},
		"reject without reason": {func(s *service) error {
			_, err := s.RejectRequest(ctx, testSellerID, testRequestID, ReasonInput{Reason: "  "})
			return err
		}, app.ErrMissingRequiredFields},
		"ask without question": {func(s *service) error {
			_, err := s.RequestInfo(ctx, testSellerID, testRequestID, ReasonInput{})
			return err
		}, app.ErrMissingRequiredFields},
		"reason too long": {func(s *service) error {
			_, err := s.RejectRequest(ctx, testSellerID, testRequestID, ReasonInput{Reason: strings.Repeat("a", maxMessageLength+1)})
			return err
		}, app.ErrInvalidInput},
	} {
		t.Run(name, func(t *testing.T) {
			st := newSetup().withRequest(StatusUnderReview)
			if err := test.act(st.s); !errors.Is(err, test.want) {
				t.Errorf("err = %v, want %v", err, test.want)
			}
			if len(st.r.changes) != 0 {
				t.Error("status changed")
			}
		})
	}
}

func TestComplementRequest(t *testing.T) {
	ctx := context.Background()

	t.Run("needs a message", func(t *testing.T) {
		st := newSetup().withRequest(StatusNeedsInfo)
		if _, err := st.s.ComplementRequest(ctx, testBuyerID, testRequestID, ComplementInput{}); !errors.Is(err, app.ErrMissingRequiredFields) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("checks new measurements", func(t *testing.T) {
		st := newSetup().withRequest(StatusNeedsInfo)
		bad := validMeasurements()
		bad.ShoeSize = 5
		if _, err := st.s.ComplementRequest(ctx, testBuyerID, testRequestID, ComplementInput{Message: "Novo", Measurements: &bad}); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("needs consent", func(t *testing.T) {
		st := newSetup().withRequest(StatusNeedsInfo)
		st.consents.granted[testBuyerID] = false
		if _, err := st.s.ComplementRequest(ctx, testBuyerID, testRequestID, ComplementInput{Message: "Novo"}); !errors.Is(err, app.ErrConsentRequired) {
			t.Errorf("err = %v", err)
		}
		if len(st.r.changes) != 0 {
			t.Error("status changed")
		}
	})
	t.Run("keeps measurements not sent", func(t *testing.T) {
		st := newSetup().withRequest(StatusNeedsInfo)
		request, err := st.s.ComplementRequest(ctx, testBuyerID, testRequestID, ComplementInput{Message: "Uso sapato 39.", TechnicalNotes: ptr("Uso diário.")})
		if err != nil {
			t.Fatalf("ComplementRequest: %v", err)
		}
		if st.r.changes[0].Measurements != nil || request.Measurements.ShoeSize != 39 || *request.TechnicalNotes != "Uso diário." {
			t.Errorf("change = %+v, request = %+v", st.r.changes[0], request)
		}
	})
}

func TestCancelRequest(t *testing.T) {
	ctx := context.Background()

	t.Run("approved, before checkout", func(t *testing.T) {
		st := newSetup().withRequest(StatusApproved)
		request, err := st.s.CancelRequest(ctx, testBuyerID, testRequestID, CancelInput{Reason: ptr("Encontrei outra opção.")})
		if err != nil || request.Status != StatusCancelled {
			t.Fatalf("CancelRequest: %v, %+v", err, request)
		}
		if n := st.r.notified[0]; n.UserID != testSellerID || n.Title != "Solicitação cancelada" {
			t.Errorf("notification = %+v", n)
		}
		if *st.r.changes[0].Reason != "Encontrei outra opção." {
			t.Errorf("reason = %v", st.r.changes[0].Reason)
		}
	})
	t.Run("not after checkout", func(t *testing.T) {
		st := newSetup().withRequest(StatusApproved)
		st.r.requests[testRequestID].OrderID = ptr("order-1")
		if _, err := st.s.CancelRequest(ctx, testBuyerID, testRequestID, CancelInput{}); !errors.Is(err, app.ErrInvalidStatusTransition) {
			t.Errorf("err = %v", err)
		}
		if actions := st.r.requests[testRequestID].ToOutput(testBuyerID).Actions; len(actions) != 0 {
			t.Errorf("actions = %v", actions)
		}
	})
}

func TestListRequests(t *testing.T) {
	ctx := context.Background()
	st := newSetup().withRequest(StatusRequested)
	st.r.total = 21

	page, err := st.s.ListRequests(ctx, testBuyerID, StatusRequested, 2, 10)
	if err != nil {
		t.Fatalf("ListRequests: %v", err)
	}
	if st.r.filter != (Filter{BuyerID: testBuyerID, Status: StatusRequested}) || page.TotalPages != 3 || page.CurrentPage != 2 || len(page.Items) != 1 {
		t.Errorf("filter = %+v, page = %+v", st.r.filter, page)
	}

	if _, err := st.s.ListSellerRequests(ctx, testSellerID, "", 0, 0); err != nil || st.r.filter != (Filter{SellerID: testSellerID}) {
		t.Errorf("ListSellerRequests: %v, filter = %+v", err, st.r.filter)
	}
	if _, err := st.s.ListRequests(ctx, testBuyerID, "pending", 1, 10); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("unknown status err = %v", err)
	}
	if _, err := st.s.ListRequests(ctx, testBuyerID, "", 1, maxRequestsPerPage+1); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("limit err = %v", err)
	}
}
