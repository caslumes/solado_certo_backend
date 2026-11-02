package support

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"slices"
	"strings"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/documents"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

type CreateTicketUseCase interface {
	CreateTicket(ctx context.Context, userID string, input TicketInput) (*TicketDetailEntity, error)
}

type ListTicketsUseCase interface {
	ListTickets(ctx context.Context, userID string, status Status, page uint, limit uint) (*TicketPage, error)
}

type GetTicketUseCase interface {
	GetTicket(ctx context.Context, userID string, id string) (*TicketDetailEntity, error)
}

type AddMessageUseCase interface {
	AddMessage(ctx context.Context, userID string, id string, message string, attachment io.Reader) (*TicketDetailEntity, error)
}

type OpenAttachmentUseCase interface {
	OpenAttachment(ctx context.Context, userID string, id string, messageID string) (*AttachmentFile, error)
}

type RoleChecker interface {
	HasRole(ctx context.Context, userID string, role enum.Role) (bool, error)
}

type repository interface {
	GetOrderParties(ctx context.Context, orderID string) (*OrderParties, error)
	ListAdminIDs(ctx context.Context) ([]string, error)
	CreateTicket(ctx context.Context, ticket NewTicket) (string, error)
	GetTicket(ctx context.Context, id string) (*TicketDetailEntity, error)
	CountTickets(ctx context.Context, filter Filter) (uint, error)
	GetTickets(ctx context.Context, filter Filter, limit uint, offset uint) ([]*TicketEntity, error)
	AddMessage(ctx context.Context, message NewMessage) error
	RecordAttachmentView(ctx context.Context, ticketID string, messageID string, userID string) error
}

type service struct {
	r     repository
	roles RoleChecker
	files storage.Storage
}

func NewService(r repository, roles RoleChecker, files storage.Storage) *service {
	return &service{r: r, roles: roles, files: files}
}

func (s *service) CreateTicket(ctx context.Context, userID string, input TicketInput) (*TicketDetailEntity, error) {
	ticket, err := input.check()
	if err != nil {
		return nil, err
	}

	var order *OrderParties
	if ticket.OrderID != nil {
		if !helpers.IsUUID(*ticket.OrderID) {
			return nil, app.ErrOrderNotFound
		}
		order, err = s.r.GetOrderParties(ctx, *ticket.OrderID)
		if err != nil {
			if !errors.Is(err, app.ErrOrderNotFound) {
				slog.ErrorContext(ctx, "GetOrderParties failed", "order_id", *ticket.OrderID, "error", err)
			}
			return nil, err
		}
		if !order.Has(userID) {
			return nil, app.ErrOrderNotFound
		}
	}

	admins, err := s.r.ListAdminIDs(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "ListAdminIDs failed", "error", err)
		return nil, err
	}

	id, err := s.r.CreateTicket(ctx, NewTicket{
		OpenedBy:    userID,
		Category:    ticket.Category,
		Subject:     ticket.Subject,
		Description: ticket.Description,
		OrderID:     ticket.OrderID,
		Notifications: func(ticketID string) []notifications.NewNotification {
			var list []notifications.NewNotification
			for _, adminID := range admins {
				if adminID != userID {
					list = append(list, notifications.SupportTicketOpenedForSupport(adminID, ticketID, ticket.Subject))
				}
			}
			if order != nil {
				counterpart := order.SellerID
				if userID == order.SellerID {
					counterpart = order.BuyerID
				}
				if counterpart != userID {
					list = append(list, notifications.SupportTicketOpenedAboutOrder(counterpart, ticketID, order.Number, ticket.Subject))
				}
			}
			return list
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "CreateTicket failed", "error", err)
		return nil, err
	}

	return s.load(ctx, id)
}

func (s *service) ListTickets(ctx context.Context, userID string, status Status, page uint, limit uint) (*TicketPage, error) {
	if status != "" && !slices.Contains(statuses, status) {
		names := make([]string, len(statuses))
		for i, st := range statuses {
			names[i] = string(st)
		}
		return nil, fmt.Errorf("%w: status must be one of %s", app.ErrInvalidInput, strings.Join(names, ", "))
	}
	if page == 0 {
		page = 1
	}
	if limit == 0 {
		limit = defaultTicketsPerPage
	}
	if limit > maxTicketsPerPage {
		return nil, fmt.Errorf("%w: limit must be at most %d", app.ErrInvalidInput, maxTicketsPerPage)
	}

	filter := Filter{ParticipantID: userID, Status: status}
	total, err := s.r.CountTickets(ctx, filter)
	if err != nil {
		slog.ErrorContext(ctx, "CountTickets failed", "error", err)
		return nil, err
	}

	items, err := s.r.GetTickets(ctx, filter, limit, (page-1)*limit)
	if err != nil {
		slog.ErrorContext(ctx, "GetTickets failed", "error", err)
		return nil, err
	}

	return &TicketPage{Items: items, CurrentPage: page, TotalPages: (total + limit - 1) / limit, TotalItems: total}, nil
}

func (s *service) GetTicket(ctx context.Context, userID string, id string) (*TicketDetailEntity, error) {
	ticket, _, err := s.access(ctx, userID, id)
	return ticket, err
}

func (s *service) AddMessage(ctx context.Context, userID string, id string, message string, attachment io.Reader) (*TicketDetailEntity, error) {
	text, err := checkMessage(message)
	if err != nil {
		return nil, err
	}

	ticket, bySupport, err := s.access(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if !ticket.Status.TakesMessages() {
		return nil, fmt.Errorf("%w: a %s ticket takes no more messages", app.ErrInvalidStatusTransition, ticket.Status)
	}

	var key *string
	if attachment != nil {
		upload, err := storage.ReadUpload(attachment, app.MaxUploadSize, attachmentTypes)
		if err != nil {
			return nil, err
		}
		if err := documents.CheckReadable(upload); err != nil {
			return nil, err
		}
		saved, err := s.files.Save(ctx, storage.AreaPrivate, attachmentsFolder, upload.Extension, upload.Reader())
		if err != nil {
			slog.ErrorContext(ctx, "AddMessage could not save file", "ticket_id", ticket.ID, "error", err)
			return nil, err
		}
		key = &saved
	}

	recipients := ticket.messageRecipients(userID, bySupport)
	list := make([]notifications.NewNotification, len(recipients))
	for i, recipient := range recipients {
		list[i] = notifications.SupportTicketReplied(recipient, ticket.ID, ticket.Subject, bySupport)
	}

	err = s.r.AddMessage(ctx, NewMessage{
		TicketID:      ticket.ID,
		SenderID:      userID,
		Message:       text,
		AttachmentKey: key,
		From:          ticket.Status,
		To:            ticket.statusAfterMessage(bySupport),
		Notifications: list,
	})
	if err != nil {
		if !errors.Is(err, app.ErrInvalidStatusTransition) {
			slog.ErrorContext(ctx, "AddMessage failed", "ticket_id", ticket.ID, "error", err)
		}
		if key != nil {
			s.deleteFile(ctx, *key)
		}
		return nil, err
	}

	return s.load(ctx, ticket.ID)
}

func (s *service) OpenAttachment(ctx context.Context, userID string, id string, messageID string) (*AttachmentFile, error) {
	ticket, _, err := s.access(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	message := ticket.message(messageID)
	if message == nil || message.AttachmentKey == nil {
		return nil, app.ErrAttachmentNotFound
	}
	if userID != message.SenderID {
		if err := s.r.RecordAttachmentView(ctx, ticket.ID, message.ID, userID); err != nil {
			slog.ErrorContext(ctx, "RecordAttachmentView failed", "ticket_id", ticket.ID, "error", err)
			return nil, err
		}
	}

	file, err := s.files.Open(ctx, storage.AreaPrivate, *message.AttachmentKey)
	if err != nil {
		slog.ErrorContext(ctx, "OpenAttachment could not open file", "ticket_id", ticket.ID, "error", err)
		if errors.Is(err, storage.ErrNotFound) {
			return nil, app.ErrAttachmentNotFound
		}
		return nil, err
	}
	key := *message.AttachmentKey
	return &AttachmentFile{File: file, ContentType: storage.ContentType(key), Name: "anexo" + path.Ext(key)}, nil
}

func (s *service) access(ctx context.Context, userID string, id string) (*TicketDetailEntity, bool, error) {
	ticket, err := s.load(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if ticket.IsParty(userID) {
		return ticket, false, nil
	}

	admin, err := s.roles.HasRole(ctx, userID, enum.RoleAdmin)
	if err != nil {
		slog.ErrorContext(ctx, "HasRole failed", "user_id", userID, "error", err)
		return nil, false, err
	}
	if !admin {
		return nil, false, app.ErrTicketNotFound
	}
	return ticket, true, nil
}

func (s *service) load(ctx context.Context, id string) (*TicketDetailEntity, error) {
	if !helpers.IsUUID(id) {
		return nil, app.ErrTicketNotFound
	}

	ticket, err := s.r.GetTicket(ctx, id)
	if err != nil {
		if !errors.Is(err, app.ErrTicketNotFound) {
			slog.ErrorContext(ctx, "GetTicket failed", "ticket_id", id, "error", err)
		}
		return nil, err
	}
	return ticket, nil
}

func (s *service) deleteFile(ctx context.Context, key string) {
	if err := s.files.Delete(ctx, storage.AreaPrivate, key); err != nil {
		slog.ErrorContext(ctx, "could not delete file", "key", key, "error", err)
	}
}
