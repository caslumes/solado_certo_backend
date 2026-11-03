package support

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

type Status string

const (
	StatusOpen             Status = "open"
	StatusInProgress       Status = "in_progress"
	StatusAwaitingCustomer Status = "awaiting_customer"
	StatusResolved         Status = "resolved"
	StatusClosed           Status = "closed"
)

var statuses = []Status{StatusOpen, StatusInProgress, StatusAwaitingCustomer, StatusResolved, StatusClosed}

func (s Status) TakesMessages() bool {
	return s != StatusResolved && s != StatusClosed
}

type Category string

const (
	CategoryOrder      Category = "order"
	CategoryPayment    Category = "payment"
	CategoryAdaptation Category = "adaptation"
	CategoryOther      Category = "other"
)

var categories = []Category{CategoryOrder, CategoryPayment, CategoryAdaptation, CategoryOther}

func (c Category) NeedsOrder() bool {
	return c == CategoryOrder || c == CategoryPayment
}

type Role string

const (
	RoleBuyer   Role = "buyer"
	RoleSeller  Role = "seller"
	RoleSupport Role = "support"
	RoleUser    Role = "user"
)

const (
	minSubjectLength     = 5
	maxSubjectLength     = 150
	minDescriptionLength = 20
	maxDescriptionLength = 2000
	maxMessageLength     = 2000
	minResolutionLength  = 10
	maxResolutionLength  = 2000

	defaultTicketsPerPage = 10
	maxTicketsPerPage     = 100

	attachmentsFolder = "support"
)

var attachmentTypes = storage.AllowedTypes{"image/jpeg": ".jpg", "image/png": ".png", "application/pdf": ".pdf"}

type TicketInput struct {
	Category    string  `json:"category"`
	Subject     string  `json:"subject"`
	Description string  `json:"description"`
	OrderID     *string `json:"order_id"`
}

type checkedTicket struct {
	Category    Category
	Subject     string
	Description string
	OrderID     *string
}

func (in TicketInput) check() (checkedTicket, error) {
	category := Category(strings.TrimSpace(in.Category))
	subject := strings.TrimSpace(in.Subject)
	description := strings.TrimSpace(in.Description)
	var orderID *string
	if in.OrderID != nil && strings.TrimSpace(*in.OrderID) != "" {
		id := strings.TrimSpace(*in.OrderID)
		orderID = &id
	}

	var missing []string
	if category == "" {
		missing = append(missing, "category")
	}
	if subject == "" {
		missing = append(missing, "subject")
	}
	if description == "" {
		missing = append(missing, "description")
	}
	if category.NeedsOrder() && orderID == nil {
		missing = append(missing, "order_id")
	}
	if len(missing) > 0 {
		return checkedTicket{}, &app.MissingFieldsError{Fields: missing}
	}

	var invalid []string
	if !slices.Contains(categories, category) {
		names := make([]string, len(categories))
		for i, c := range categories {
			names[i] = string(c)
		}
		invalid = append(invalid, "category must be one of "+strings.Join(names, ", "))
	}
	if n := len([]rune(subject)); n < minSubjectLength || n > maxSubjectLength {
		invalid = append(invalid, fmt.Sprintf("subject must have between %d and %d characters", minSubjectLength, maxSubjectLength))
	}
	if n := len([]rune(description)); n < minDescriptionLength || n > maxDescriptionLength {
		invalid = append(invalid, fmt.Sprintf("description must have between %d and %d characters", minDescriptionLength, maxDescriptionLength))
	}
	if len(invalid) > 0 {
		return checkedTicket{}, fmt.Errorf("%w: %s", app.ErrInvalidInput, strings.Join(invalid, "; "))
	}

	return checkedTicket{Category: category, Subject: subject, Description: description, OrderID: orderID}, nil
}

func checkMessage(message string) (string, error) {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "", &app.MissingFieldsError{Fields: []string{"message"}}
	}
	if len([]rune(trimmed)) > maxMessageLength {
		return "", fmt.Errorf("%w: message must have at most %d characters", app.ErrInvalidInput, maxMessageLength)
	}
	return trimmed, nil
}

func checkResolution(resolution string) (string, error) {
	trimmed := strings.TrimSpace(resolution)
	if trimmed == "" {
		return "", &app.MissingFieldsError{Fields: []string{"resolution"}}
	}
	if n := len([]rune(trimmed)); n < minResolutionLength || n > maxResolutionLength {
		return "", fmt.Errorf("%w: resolution must have between %d and %d characters", app.ErrInvalidInput, minResolutionLength, maxResolutionLength)
	}
	return trimmed, nil
}

type OrderParties struct {
	ID       string
	Number   string
	BuyerID  string
	SellerID string
}

func (o *OrderParties) Has(userID string) bool {
	return o.BuyerID == userID || o.SellerID == userID
}

type NewTicket struct {
	OpenedBy      string
	Category      Category
	Subject       string
	Description   string
	OrderID       *string
	Notifications func(ticketID string) []notifications.NewNotification
}

type NewMessage struct {
	TicketID      string
	SenderID      string
	Message       string
	AttachmentKey *string
	From          Status
	To            Status
	Notifications []notifications.NewNotification
}

type TicketChange struct {
	TicketID      string
	ActorID       string
	From          Status
	To            Status
	AssignTo      *string
	Resolution    *string
	Action        string
	Details       map[string]any
	Notifications []notifications.NewNotification
}

type TicketEntity struct {
	ID          string
	Category    Category
	Status      Status
	Subject     string
	Description string
	OpenedBy    string
	OpenerName  string
	AssignedTo  *string
	Assignee    *string
	Resolution  *string
	Order       *OrderParties
	OpenedAt    time.Time
	UpdatedAt   time.Time
	ClosedAt    *time.Time
}

func (t *TicketEntity) parties() []string {
	parties := []string{t.OpenedBy}
	if t.Order != nil {
		for _, id := range []string{t.Order.BuyerID, t.Order.SellerID} {
			if !slices.Contains(parties, id) {
				parties = append(parties, id)
			}
		}
	}
	return parties
}

func (t *TicketEntity) IsParty(userID string) bool {
	return slices.Contains(t.parties(), userID)
}

func (t *TicketEntity) roleOf(userID string, isAdmin bool) Role {
	if t.Order != nil {
		switch userID {
		case t.Order.BuyerID:
			return RoleBuyer
		case t.Order.SellerID:
			return RoleSeller
		}
	}
	if isAdmin && userID != t.OpenedBy {
		return RoleSupport
	}
	return RoleUser
}

func (t *TicketEntity) statusAfterMessage(bySupport bool) Status {
	if bySupport {
		if t.Status == StatusOpen || t.Status == StatusInProgress {
			return StatusAwaitingCustomer
		}
		return t.Status
	}
	if t.Status == StatusAwaitingCustomer {
		if t.AssignedTo != nil {
			return StatusInProgress
		}
		return StatusOpen
	}
	return t.Status
}

func (t *TicketEntity) messageRecipients(senderID string, bySupport bool) []string {
	var recipients []string
	for _, id := range t.parties() {
		if id != senderID {
			recipients = append(recipients, id)
		}
	}
	if !bySupport && t.AssignedTo != nil && *t.AssignedTo != senderID && !slices.Contains(recipients, *t.AssignedTo) {
		recipients = append(recipients, *t.AssignedTo)
	}
	return recipients
}

func (t *TicketEntity) ToSummaryOutput(viewerID string) TicketSummaryOutput {
	var orderID, orderNumber *string
	if t.Order != nil {
		orderID, orderNumber = &t.Order.ID, &t.Order.Number
	}
	return TicketSummaryOutput{
		ID:           t.ID,
		Category:     string(t.Category),
		Status:       string(t.Status),
		Subject:      t.Subject,
		OrderID:      orderID,
		OrderNumber:  orderNumber,
		OpenedByMe:   t.OpenedBy == viewerID,
		AssignedToMe: t.AssignedTo != nil && *t.AssignedTo == viewerID,
		OpenedAt:     t.OpenedAt,
		UpdatedAt:    t.UpdatedAt,
		ClosedAt:     t.ClosedAt,
	}
}

type MessageEntity struct {
	ID            string
	SenderID      string
	SenderName    string
	SenderIsAdmin bool
	Message       string
	AttachmentKey *string
	CreatedAt     time.Time
}

type TicketDetailEntity struct {
	TicketEntity
	Messages []MessageEntity
}

func (d *TicketDetailEntity) message(id string) *MessageEntity {
	for i := range d.Messages {
		if d.Messages[i].ID == id {
			return &d.Messages[i]
		}
	}
	return nil
}

func AttachmentPath(ticketID string, messageID string) string {
	return "/support/tickets/" + ticketID + "/messages/" + messageID + "/attachment"
}

func (d *TicketDetailEntity) ToOutput(viewerID string) *TicketDetailOutput {
	messages := make([]MessageOutput, len(d.Messages))
	for i, m := range d.Messages {
		var attachment *string
		if m.AttachmentKey != nil {
			path := AttachmentPath(d.ID, m.ID)
			attachment = &path
		}
		role := d.roleOf(m.SenderID, m.SenderIsAdmin)
		name := m.SenderName
		if role == RoleSupport {
			name = "Suporte Solado Certo"
		}
		messages[i] = MessageOutput{
			ID:            m.ID,
			SenderRole:    string(role),
			SenderName:    name,
			Mine:          m.SenderID == viewerID,
			Message:       m.Message,
			AttachmentURL: attachment,
			CreatedAt:     m.CreatedAt,
		}
	}
	return &TicketDetailOutput{
		TicketSummaryOutput: d.ToSummaryOutput(viewerID),
		Description:         d.Description,
		Resolution:          d.Resolution,
		CanReply:            d.Status.TakesMessages(),
		Messages:            messages,
	}
}

type AttachmentFile struct {
	*storage.File
	ContentType string
	Name        string
}

type QueueQuery struct {
	Status   string
	Assigned string
}

const (
	queueActive      Status = "active"
	assignedToMe            = "me"
	assignedToNobody        = "none"
)

type Filter struct {
	ParticipantID string
	Status        Status
	Active        bool
	AssignedTo    string
	Unassigned    bool
}

type TicketPage struct {
	Items       []*TicketEntity
	CurrentPage uint
	TotalPages  uint
	TotalItems  uint
}

func (p *TicketPage) ToOutput(viewerID string) *TicketPageOutput {
	items := make([]TicketSummaryOutput, len(p.Items))
	for i, ticket := range p.Items {
		items[i] = ticket.ToSummaryOutput(viewerID)
	}
	return &TicketPageOutput{
		Items:       items,
		CurrentPage: p.CurrentPage,
		TotalPages:  p.TotalPages,
		TotalItems:  p.TotalItems,
		HasNext:     p.CurrentPage < p.TotalPages,
		HasPrevious: p.CurrentPage > 1,
	}
}

func (t *TicketEntity) ToQueueOutput(viewerID string) QueueItemOutput {
	item := QueueItemOutput{
		TicketSummaryOutput: t.ToSummaryOutput(viewerID),
		OpenedBy:            PersonOutput{ID: t.OpenedBy, Name: t.OpenerName},
	}
	if t.AssignedTo != nil {
		name := ""
		if t.Assignee != nil {
			name = *t.Assignee
		}
		item.AssignedTo = &PersonOutput{ID: *t.AssignedTo, Name: name}
	}
	return item
}

func (p *TicketPage) ToQueueOutput(viewerID string) *QueuePageOutput {
	items := make([]QueueItemOutput, len(p.Items))
	for i, ticket := range p.Items {
		items[i] = ticket.ToQueueOutput(viewerID)
	}
	return &QueuePageOutput{
		Items:       items,
		CurrentPage: p.CurrentPage,
		TotalPages:  p.TotalPages,
		TotalItems:  p.TotalItems,
		HasNext:     p.CurrentPage < p.TotalPages,
		HasPrevious: p.CurrentPage > 1,
	}
}
