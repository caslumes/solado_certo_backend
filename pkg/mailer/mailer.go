package mailer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

const sendTimeout = 10 * time.Second

var ErrInvalidAddress = errors.New("invalid e-mail address")

type Message struct {
	To      string
	Subject string
	Body    string
}

type Mailer interface {
	Send(ctx context.Context, message Message) error
}

type SMTP struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	now      func() time.Time
}

func NewSMTP(host, port, username, password, from string) *SMTP {
	return &SMTP{Host: host, Port: port, Username: username, Password: password, From: from, now: time.Now}
}

func (s *SMTP) Send(ctx context.Context, message Message) error {
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("%w: from: %v", ErrInvalidAddress, err)
	}
	to, err := mail.ParseAddress(message.To)
	if err != nil {
		return fmt.Errorf("%w: to: %v", ErrInvalidAddress, err)
	}

	data, err := s.build(from, to, message)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.Host, s.Port))
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	}

	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer client.Close()

	if s.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return err
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return err
	}
	if err := client.Rcpt(to.Address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	return client.Quit()
}

func (s *SMTP) build(from, to *mail.Address, message Message) ([]byte, error) {
	var body bytes.Buffer
	encoder := quotedprintable.NewWriter(&body)
	if _, err := encoder.Write([]byte(strings.ReplaceAll(message.Body, "\n", "\r\n"))); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}

	var data bytes.Buffer
	headers := [][2]string{
		{"From", from.String()},
		{"To", to.String()},
		{"Subject", mime.QEncoding.Encode("utf-8", message.Subject)},
		{"Date", s.now().Format(time.RFC1123Z)},
		{"MIME-Version", "1.0"},
		{"Content-Type", "text/plain; charset=utf-8"},
		{"Content-Transfer-Encoding", "quoted-printable"},
	}
	for _, header := range headers {
		fmt.Fprintf(&data, "%s: %s\r\n", header[0], header[1])
	}
	data.WriteString("\r\n")
	data.Write(body.Bytes())

	return data.Bytes(), nil
}

type Disabled struct{}

func (Disabled) Send(ctx context.Context, message Message) error {
	slog.WarnContext(ctx, "e-mail not sent: SMTP_HOST is not set", "subject", message.Subject)
	return nil
}
