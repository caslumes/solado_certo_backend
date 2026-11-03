package support

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

const (
	buyerID    = "buyer-1"
	sellerID   = "seller-1"
	adminID    = "admin-1"
	strangerID = "stranger-1"
	orderID    = "11111111-1111-1111-1111-111111111111"
	ticketID   = "22222222-2222-2222-2222-222222222222"
)

type fakeRepository struct {
	orders        map[string]*OrderParties
	admins        []string
	tickets       map[string]*TicketDetailEntity
	notifications []notifications.NewNotification
	views         []string
	changes       []TicketChange
	addErr        error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		orders:  map[string]*OrderParties{orderID: {ID: orderID, Number: "SC-1", BuyerID: buyerID, SellerID: sellerID}},
		admins:  []string{adminID},
		tickets: map[string]*TicketDetailEntity{},
	}
}

func (f *fakeRepository) GetOrderParties(_ context.Context, id string) (*OrderParties, error) {
	order, ok := f.orders[id]
	if !ok {
		return nil, app.ErrOrderNotFound
	}
	return order, nil
}

func (f *fakeRepository) ListAdminIDs(context.Context) ([]string, error) {
	return f.admins, nil
}

func (f *fakeRepository) CreateTicket(_ context.Context, ticket NewTicket) (string, error) {
	now := time.Now()
	entity := &TicketDetailEntity{TicketEntity: TicketEntity{
		ID: ticketID, Category: ticket.Category, Status: StatusOpen, Subject: ticket.Subject, Description: ticket.Description,
		OpenedBy: ticket.OpenedBy, OpenedAt: now, UpdatedAt: now,
	}}
	if ticket.OrderID != nil {
		entity.Order = f.orders[*ticket.OrderID]
	}
	f.tickets[ticketID] = entity
	f.notifications = append(f.notifications, ticket.Notifications(ticketID)...)
	return ticketID, nil
}

func (f *fakeRepository) GetTicket(_ context.Context, id string) (*TicketDetailEntity, error) {
	ticket, ok := f.tickets[id]
	if !ok {
		return nil, app.ErrTicketNotFound
	}
	copied := *ticket
	copied.Messages = slices.Clone(ticket.Messages)
	return &copied, nil
}

func (f *fakeRepository) CountTickets(_ context.Context, filter Filter) (uint, error) {
	tickets, _ := f.GetTickets(context.Background(), filter, 100, 0)
	return uint(len(tickets)), nil
}

func (f *fakeRepository) GetTickets(_ context.Context, filter Filter, limit uint, offset uint) ([]*TicketEntity, error) {
	var list []*TicketEntity
	for _, ticket := range f.tickets {
		if (filter.ParticipantID == "" || ticket.IsParty(filter.ParticipantID)) && (filter.Status == "" || ticket.Status == filter.Status) &&
			(!filter.Active || ticket.Status.TakesMessages()) && (!filter.Unassigned || ticket.AssignedTo == nil) &&
			(filter.AssignedTo == "" || (ticket.AssignedTo != nil && *ticket.AssignedTo == filter.AssignedTo)) {
			list = append(list, &ticket.TicketEntity)
		}
	}
	return list, nil
}

func (f *fakeRepository) AddMessage(_ context.Context, message NewMessage) error {
	if f.addErr != nil {
		return f.addErr
	}
	ticket := f.tickets[message.TicketID]
	if ticket.Status != message.From {
		return app.ErrInvalidStatusTransition
	}
	ticket.Status = message.To
	ticket.Messages = append(ticket.Messages, MessageEntity{
		ID: "message-" + string(rune('a'+len(ticket.Messages))), SenderID: message.SenderID, SenderIsAdmin: message.SenderID == adminID,
		Message: message.Message, AttachmentKey: message.AttachmentKey, CreatedAt: time.Now(),
	})
	f.notifications = append(f.notifications, message.Notifications...)
	return nil
}

func (f *fakeRepository) ChangeTicket(_ context.Context, change TicketChange) error {
	ticket := f.tickets[change.TicketID]
	if ticket.Status != change.From {
		return app.ErrInvalidStatusTransition
	}
	ticket.Status = change.To
	if change.AssignTo != nil {
		ticket.AssignedTo = change.AssignTo
	}
	if change.Resolution != nil {
		ticket.Resolution = change.Resolution
	}
	f.changes = append(f.changes, change)
	f.notifications = append(f.notifications, change.Notifications...)
	return nil
}

func (f *fakeRepository) RecordAttachmentView(_ context.Context, _ string, messageID string, userID string) error {
	f.views = append(f.views, userID+":"+messageID)
	return nil
}

type fakeRoles struct{}

func (fakeRoles) HasRole(_ context.Context, userID string, role enum.Role) (bool, error) {
	return role == enum.RoleAdmin && userID == adminID, nil
}

func newTestService() (*service, *fakeRepository, *storage.MemoryStorage) {
	repository := newFakeRepository()
	files := storage.NewMemoryStorage()
	return NewService(repository, fakeRoles{}, files), repository, files
}

func orderTicket() TicketInput {
	id := orderID
	return TicketInput{Category: "order", Subject: "Produto errado", Description: "Recebi a palmilha no tamanho 38, pedi 40.", OrderID: &id}
}

func openTicket(t *testing.T, s *service, userID string, input TicketInput) *TicketDetailEntity {
	t.Helper()
	ticket, err := s.CreateTicket(context.Background(), userID, input)
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	return ticket
}

func recipients(list []notifications.NewNotification, kind notifications.Type) []string {
	var ids []string
	for _, n := range list {
		if n.Type == kind {
			ids = append(ids, n.UserID)
		}
	}
	slices.Sort(ids)
	return ids
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewGray(image.Rect(0, 0, 400, 400))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestCreateTicketAboutOrderNotifiesSellerAndSupport(t *testing.T) {
	s, repository, _ := newTestService()

	ticket := openTicket(t, s, buyerID, orderTicket())

	if ticket.Status != StatusOpen || ticket.Category != CategoryOrder || ticket.Order == nil || ticket.Order.ID != orderID {
		t.Fatalf("ticket = %+v", ticket)
	}
	if got := recipients(repository.notifications, notifications.TypeSupportTicketOpened); !slices.Equal(got, []string{adminID, sellerID}) {
		t.Fatalf("notified = %v, want admin and seller", got)
	}
}

func TestCreateTicketBySellerNotifiesBuyer(t *testing.T) {
	s, repository, _ := newTestService()

	openTicket(t, s, sellerID, orderTicket())

	if got := recipients(repository.notifications, notifications.TypeSupportTicketOpened); !slices.Equal(got, []string{adminID, buyerID}) {
		t.Fatalf("notified = %v, want admin and buyer", got)
	}
}

func TestCreateTicketWithoutOrder(t *testing.T) {
	s, repository, _ := newTestService()

	ticket := openTicket(t, s, buyerID, TicketInput{Category: "other", Subject: "Dúvida geral", Description: "Como funciona a troca de produtos?"})

	if ticket.Order != nil {
		t.Fatalf("order = %+v", ticket.Order)
	}
	if got := recipients(repository.notifications, notifications.TypeSupportTicketOpened); !slices.Equal(got, []string{adminID}) {
		t.Fatalf("notified = %v, want only the admin", got)
	}
}

func TestCreateTicketAsksForMissingData(t *testing.T) {
	tests := map[string]struct {
		mutate func(*TicketInput)
		fields []string
	}{
		"empty":            {func(in *TicketInput) { *in = TicketInput{} }, []string{"category", "subject", "description"}},
		"order without id": {func(in *TicketInput) { in.OrderID = nil }, []string{"order_id"}},
		"payment blank id": {func(in *TicketInput) { blank := " "; in.Category = "payment"; in.OrderID = &blank }, []string{"order_id"}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s, _, _ := newTestService()
			input := orderTicket()
			tt.mutate(&input)

			_, err := s.CreateTicket(context.Background(), buyerID, input)
			var missing *app.MissingFieldsError
			if !errors.As(err, &missing) || !slices.Equal(missing.Fields, tt.fields) {
				t.Fatalf("err = %v, want missing %v", err, tt.fields)
			}
		})
	}
}

func TestCreateTicketRejectsInvalidData(t *testing.T) {
	tests := map[string]func(*TicketInput){
		"category":          func(in *TicketInput) { in.Category = "refund" },
		"short subject":     func(in *TicketInput) { in.Subject = "Oi" },
		"long subject":      func(in *TicketInput) { in.Subject = strings.Repeat("a", maxSubjectLength+1) },
		"short description": func(in *TicketInput) { in.Description = "Deu errado." },
		"long description":  func(in *TicketInput) { in.Description = strings.Repeat("a", maxDescriptionLength+1) },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			s, _, _ := newTestService()
			input := orderTicket()
			mutate(&input)

			if _, err := s.CreateTicket(context.Background(), buyerID, input); !errors.Is(err, app.ErrInvalidInput) {
				t.Fatalf("err = %v, want %v", err, app.ErrInvalidInput)
			}
		})
	}
}

func TestCreateTicketRejectsOrderOfAnotherUser(t *testing.T) {
	for name, id := range map[string]string{"another user's": orderID, "unknown": "33333333-3333-3333-3333-333333333333", "not a uuid": "abc"} {
		t.Run(name, func(t *testing.T) {
			s, _, _ := newTestService()
			input := orderTicket()
			input.OrderID = &id

			if _, err := s.CreateTicket(context.Background(), strangerID, input); !errors.Is(err, app.ErrOrderNotFound) {
				t.Fatalf("err = %v, want %v", err, app.ErrOrderNotFound)
			}
		})
	}
}

func TestGetTicketIsLimitedToPartiesAndAdmins(t *testing.T) {
	s, _, _ := newTestService()
	openTicket(t, s, buyerID, orderTicket())

	for _, userID := range []string{buyerID, sellerID, adminID} {
		if _, err := s.GetTicket(context.Background(), userID, ticketID); err != nil {
			t.Errorf("%s: %v", userID, err)
		}
	}
	if _, err := s.GetTicket(context.Background(), strangerID, ticketID); !errors.Is(err, app.ErrTicketNotFound) {
		t.Fatalf("stranger: err = %v, want %v", err, app.ErrTicketNotFound)
	}
	if _, err := s.GetTicket(context.Background(), buyerID, "not-a-uuid"); !errors.Is(err, app.ErrTicketNotFound) {
		t.Fatalf("bad id: err = %v, want %v", err, app.ErrTicketNotFound)
	}
}

func TestListTicketsValidatesQuery(t *testing.T) {
	s, _, _ := newTestService()
	openTicket(t, s, buyerID, orderTicket())

	page, err := s.ListTickets(context.Background(), sellerID, "", 0, 0)
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	if page.TotalItems != 1 || page.CurrentPage != 1 || page.TotalPages != 1 {
		t.Fatalf("page = %+v", page)
	}

	if _, err := s.ListTickets(context.Background(), buyerID, "pending", 1, 10); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("bad status: err = %v", err)
	}
	if _, err := s.ListTickets(context.Background(), buyerID, "", 1, maxTicketsPerPage+1); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("big limit: err = %v", err)
	}
}

func TestSupportReplyWaitsForCustomerAndPartyAnswerReopens(t *testing.T) {
	s, repository, _ := newTestService()
	openTicket(t, s, buyerID, orderTicket())
	repository.notifications = nil

	ticket, err := s.AddMessage(context.Background(), adminID, ticketID, "Pode enviar uma foto da etiqueta?", nil)
	if err != nil {
		t.Fatalf("admin message: %v", err)
	}
	if ticket.Status != StatusAwaitingCustomer {
		t.Fatalf("status = %s, want %s", ticket.Status, StatusAwaitingCustomer)
	}
	if got := recipients(repository.notifications, notifications.TypeSupportTicketUpdated); !slices.Equal(got, []string{buyerID, sellerID}) {
		t.Fatalf("notified = %v, want buyer and seller", got)
	}
	if role := ticket.roleOf(adminID, true); role != RoleSupport {
		t.Fatalf("admin role = %s", role)
	}

	repository.notifications = nil
	ticket, err = s.AddMessage(context.Background(), buyerID, ticketID, "Segue a foto.", nil)
	if err != nil {
		t.Fatalf("buyer message: %v", err)
	}
	if ticket.Status != StatusOpen {
		t.Fatalf("status = %s, want %s", ticket.Status, StatusOpen)
	}
	if got := recipients(repository.notifications, notifications.TypeSupportTicketUpdated); !slices.Equal(got, []string{sellerID}) {
		t.Fatalf("notified = %v, want only the seller (no admin assigned)", got)
	}
}

func TestPartyAnswerGoesBackInProgressAndNotifiesAssignedAdmin(t *testing.T) {
	s, repository, _ := newTestService()
	openTicket(t, s, buyerID, orderTicket())
	assigned := adminID
	repository.tickets[ticketID].AssignedTo = &assigned
	repository.tickets[ticketID].Status = StatusAwaitingCustomer
	repository.notifications = nil

	ticket, err := s.AddMessage(context.Background(), sellerID, ticketID, "Já enviei outra palmilha.", nil)
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	if ticket.Status != StatusInProgress {
		t.Fatalf("status = %s, want %s", ticket.Status, StatusInProgress)
	}
	if got := recipients(repository.notifications, notifications.TypeSupportTicketUpdated); !slices.Equal(got, []string{adminID, buyerID}) {
		t.Fatalf("notified = %v, want admin and buyer", got)
	}
}

func TestAddMessageRefusals(t *testing.T) {
	s, repository, _ := newTestService()
	openTicket(t, s, buyerID, orderTicket())

	var missing *app.MissingFieldsError
	if _, err := s.AddMessage(context.Background(), buyerID, ticketID, "  ", nil); !errors.As(err, &missing) {
		t.Fatalf("blank: err = %v", err)
	}
	if _, err := s.AddMessage(context.Background(), buyerID, ticketID, strings.Repeat("a", maxMessageLength+1), nil); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("long: err = %v", err)
	}
	if _, err := s.AddMessage(context.Background(), strangerID, ticketID, "Olá", nil); !errors.Is(err, app.ErrTicketNotFound) {
		t.Fatalf("stranger: err = %v", err)
	}

	repository.tickets[ticketID].Status = StatusResolved
	if _, err := s.AddMessage(context.Background(), buyerID, ticketID, "Olá", nil); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("resolved: err = %v", err)
	}
}

func TestAddMessageWithAttachment(t *testing.T) {
	s, repository, files := newTestService()
	openTicket(t, s, buyerID, orderTicket())

	ticket, err := s.AddMessage(context.Background(), buyerID, ticketID, "Foto da etiqueta.", bytes.NewReader(testPNG(t)))
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	message := ticket.Messages[0]
	if message.AttachmentKey == nil || !files.Has(storage.AreaPrivate, *message.AttachmentKey) || !strings.HasPrefix(*message.AttachmentKey, attachmentsFolder+"/") {
		t.Fatalf("attachment = %v", message.AttachmentKey)
	}

	file, err := s.OpenAttachment(context.Background(), buyerID, ticketID, message.ID)
	if err != nil {
		t.Fatalf("OpenAttachment (sender): %v", err)
	}
	file.Close()
	if file.ContentType != "image/png" || file.Name != "anexo.png" || len(repository.views) != 0 {
		t.Fatalf("file = %+v, views = %v", file, repository.views)
	}

	file, err = s.OpenAttachment(context.Background(), adminID, ticketID, message.ID)
	if err != nil {
		t.Fatalf("OpenAttachment (admin): %v", err)
	}
	file.Close()
	if !slices.Equal(repository.views, []string{adminID + ":" + message.ID}) {
		t.Fatalf("views = %v", repository.views)
	}

	if _, err := s.OpenAttachment(context.Background(), strangerID, ticketID, message.ID); !errors.Is(err, app.ErrTicketNotFound) {
		t.Fatalf("stranger: err = %v", err)
	}
	if _, err := s.OpenAttachment(context.Background(), buyerID, ticketID, "unknown"); !errors.Is(err, app.ErrAttachmentNotFound) {
		t.Fatalf("unknown message: err = %v", err)
	}
}

func TestAddMessageRejectsBadAttachments(t *testing.T) {
	tests := map[string]struct {
		content []byte
		want    error
	}{
		"text":         {[]byte("just some text"), storage.ErrUnsupportedType},
		"too large":    {make([]byte, app.MaxUploadSize+1), storage.ErrTooLarge},
		"broken PDF":   {[]byte("%PDF-1.4 truncated"), app.ErrUnreadableDocument},
		"tiny picture": {nil, app.ErrUnreadableDocument},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s, _, files := newTestService()
			openTicket(t, s, buyerID, orderTicket())
			content := tt.content
			if content == nil {
				var buffer bytes.Buffer
				png.Encode(&buffer, image.NewGray(image.Rect(0, 0, 10, 10)))
				content = buffer.Bytes()
			}

			if _, err := s.AddMessage(context.Background(), buyerID, ticketID, "Anexo", bytes.NewReader(content)); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if files.Count() != 0 {
				t.Fatalf("a refused file was stored")
			}
		})
	}
}

func TestAddMessageDeletesAttachmentWhenSavingFails(t *testing.T) {
	s, repository, files := newTestService()
	openTicket(t, s, buyerID, orderTicket())
	repository.addErr = errors.New("database down")

	if _, err := s.AddMessage(context.Background(), buyerID, ticketID, "Foto", bytes.NewReader(testPNG(t))); err == nil {
		t.Fatal("AddMessage succeeded")
	}
	if files.Count() != 0 {
		t.Fatal("the attachment was kept")
	}
}

func TestTicketOutputShowsRolesFromViewer(t *testing.T) {
	ticket := &TicketDetailEntity{
		TicketEntity: TicketEntity{ID: ticketID, Status: StatusOpen, OpenedBy: buyerID, Order: &OrderParties{ID: orderID, Number: "SC-1", BuyerID: buyerID, SellerID: sellerID}},
		Messages: []MessageEntity{
			{ID: "m1", SenderID: buyerID, SenderName: "Ana", Message: "a"},
			{ID: "m2", SenderID: adminID, SenderName: "Admin Pessoa", SenderIsAdmin: true, Message: "b"},
			{ID: "m3", SenderID: sellerID, SenderName: "Loja", Message: "c", AttachmentKey: new(string)},
		},
	}

	out := ticket.ToOutput(sellerID)

	if out.OpenedByMe || !out.CanReply || *out.OrderNumber != "SC-1" {
		t.Fatalf("out = %+v", out)
	}
	want := []struct {
		role string
		name string
		mine bool
	}{{"buyer", "Ana", false}, {"support", "Suporte Solado Certo", false}, {"seller", "Loja", true}}
	for i, w := range want {
		m := out.Messages[i]
		if m.SenderRole != w.role || m.SenderName != w.name || m.Mine != w.mine {
			t.Errorf("message %d = %+v, want %+v", i, m, w)
		}
	}
	if out.Messages[2].AttachmentURL == nil || *out.Messages[2].AttachmentURL != "/support/tickets/"+ticketID+"/messages/m3/attachment" {
		t.Fatalf("attachment url = %v", out.Messages[2].AttachmentURL)
	}
}

func TestListQueueFilters(t *testing.T) {
	s, repository, _ := newTestService()
	mine := openTicket(t, s, buyerID, orderTicket())
	repository.tickets[mine.ID].AssignedTo = new(string)
	*repository.tickets[mine.ID].AssignedTo = adminID
	repository.tickets["33333333-3333-3333-3333-333333333333"] = &TicketDetailEntity{TicketEntity: TicketEntity{
		ID: "33333333-3333-3333-3333-333333333333", Status: StatusResolved, OpenedBy: strangerID,
	}}
	ctx := context.Background()

	tests := map[string]struct {
		query QueueQuery
		want  []string
	}{
		"everything": {QueueQuery{}, []string{mine.ID, "33333333-3333-3333-3333-333333333333"}},
		"active":     {QueueQuery{Status: "active"}, []string{mine.ID}},
		"resolved":   {QueueQuery{Status: "resolved"}, []string{"33333333-3333-3333-3333-333333333333"}},
		"mine":       {QueueQuery{Assigned: "me"}, []string{mine.ID}},
		"nobody":     {QueueQuery{Assigned: "none"}, []string{"33333333-3333-3333-3333-333333333333"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			page, err := s.ListQueue(ctx, adminID, tt.query, 1, 10)
			if err != nil {
				t.Fatalf("ListQueue: %v", err)
			}
			var ids []string
			for _, item := range page.Items {
				ids = append(ids, item.ID)
			}
			slices.Sort(ids)
			if !slices.Equal(ids, tt.want) {
				t.Fatalf("ids = %v, want %v", ids, tt.want)
			}
		})
	}

	for _, query := range []QueueQuery{{Status: "lost"}, {Assigned: "someone"}} {
		if _, err := s.ListQueue(ctx, adminID, query, 1, 10); !errors.Is(err, app.ErrInvalidInput) {
			t.Fatalf("%+v: err = %v", query, err)
		}
	}
}

func TestAssignTicketTakesItInProgress(t *testing.T) {
	s, repository, _ := newTestService()
	ticket := openTicket(t, s, buyerID, orderTicket())

	assigned, err := s.AssignTicket(context.Background(), adminID, ticket.ID)
	if err != nil {
		t.Fatalf("AssignTicket: %v", err)
	}
	if assigned.Status != StatusInProgress || assigned.AssignedTo == nil || *assigned.AssignedTo != adminID {
		t.Fatalf("ticket = %+v", assigned.TicketEntity)
	}
	change := repository.changes[0]
	if change.Action != "support.ticket_assigned" || change.ActorID != adminID || change.From != StatusOpen {
		t.Fatalf("change = %+v", change)
	}

	if _, err := s.AssignTicket(context.Background(), adminID, ticket.ID); err != nil || len(repository.changes) != 1 {
		t.Fatalf("assigning again: err = %v, changes = %d", err, len(repository.changes))
	}

	repository.tickets[ticket.ID].Status = StatusAwaitingCustomer
	other := "admin-2"
	if _, err := s.AssignTicket(context.Background(), other, ticket.ID); err != nil {
		t.Fatalf("taking over: %v", err)
	}
	if got := repository.tickets[ticket.ID]; got.Status != StatusAwaitingCustomer || *got.AssignedTo != other {
		t.Fatalf("taken over = %+v", got.TicketEntity)
	}

	repository.tickets[ticket.ID].Status = StatusResolved
	if _, err := s.AssignTicket(context.Background(), adminID, ticket.ID); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("resolved: err = %v", err)
	}
	if _, err := s.AssignTicket(context.Background(), adminID, "not-a-uuid"); !errors.Is(err, app.ErrTicketNotFound) {
		t.Fatalf("unknown: err = %v", err)
	}
}

func TestCloseTicketRecordsResolutionAndNotifiesParties(t *testing.T) {
	s, repository, _ := newTestService()
	ticket := openTicket(t, s, buyerID, orderTicket())
	repository.notifications = nil

	closed, err := s.CloseTicket(context.Background(), adminID, ticket.ID, "  Reembolso parcial aprovado pelo vendedor.  ")
	if err != nil {
		t.Fatalf("CloseTicket: %v", err)
	}
	if closed.Status != StatusResolved || closed.Resolution == nil || *closed.Resolution != "Reembolso parcial aprovado pelo vendedor." {
		t.Fatalf("ticket = %+v", closed.TicketEntity)
	}
	if closed.AssignedTo == nil || *closed.AssignedTo != adminID {
		t.Fatalf("an unassigned ticket should be assigned to whoever closes it: %+v", closed.AssignedTo)
	}
	output := closed.ToOutput(buyerID)
	if output.CanReply || output.Resolution == nil {
		t.Fatalf("output = %+v", output)
	}

	var recipients []string
	for _, n := range repository.notifications {
		recipients = append(recipients, n.UserID)
		if n.Type != notifications.TypeSupportTicketUpdated || !strings.Contains(n.Body, "Reembolso parcial") {
			t.Fatalf("notification = %+v", n)
		}
	}
	slices.Sort(recipients)
	if !slices.Equal(recipients, []string{buyerID, sellerID}) {
		t.Fatalf("recipients = %v", recipients)
	}
	if change := repository.changes[0]; change.Action != "support.ticket_resolved" || change.Details["resolution"] != "Reembolso parcial aprovado pelo vendedor." {
		t.Fatalf("change = %+v", change)
	}

	if _, err := s.CloseTicket(context.Background(), adminID, ticket.ID, "Encerrando de novo o chamado."); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("closing twice: err = %v", err)
	}
}

func TestCloseTicketNeedsResolution(t *testing.T) {
	s, _, _ := newTestService()
	ticket := openTicket(t, s, buyerID, orderTicket())

	var missing *app.MissingFieldsError
	if _, err := s.CloseTicket(context.Background(), adminID, ticket.ID, "  "); !errors.As(err, &missing) {
		t.Fatalf("empty: err = %v", err)
	}
	if _, err := s.CloseTicket(context.Background(), adminID, ticket.ID, "Ok."); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("short: err = %v", err)
	}
}
