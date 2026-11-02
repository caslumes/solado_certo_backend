package mailer

import (
	"context"
	"errors"
	"io"
	"mime/quotedprintable"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

type received struct {
	from string
	to   string
	data string
}

// serveSMTP accepts one connection and answers just enough of SMTP for
// net/smtp, returning what the client sent.
func serveSMTP(t *testing.T) (host, port string, result <-chan received) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	out := make(chan received, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		text := textproto.NewConn(conn)
		var got received
		text.PrintfLine("220 test ready")
		for {
			line, err := text.ReadLine()
			if err != nil {
				return
			}
			command := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
				text.PrintfLine("250 test")
			case strings.HasPrefix(command, "MAIL FROM:"):
				got.from = line[len("MAIL FROM:"):]
				text.PrintfLine("250 ok")
			case strings.HasPrefix(command, "RCPT TO:"):
				got.to = line[len("RCPT TO:"):]
				text.PrintfLine("250 ok")
			case command == "DATA":
				text.PrintfLine("354 go ahead")
				data, err := io.ReadAll(text.DotReader())
				if err != nil {
					return
				}
				got.data = string(data)
				text.PrintfLine("250 queued")
			case command == "QUIT":
				text.PrintfLine("221 bye")
				out <- got
				return
			default:
				text.PrintfLine("502 not implemented")
			}
		}
	}()

	host, port, _ = net.SplitHostPort(listener.Addr().String())
	return host, port, out
}

func TestSMTPSendsMessage(t *testing.T) {
	host, port, result := serveSMTP(t)
	sender := NewSMTP(host, port, "", "", "Solado Certo <nao-responda@soladocerto.local>")
	sender.now = func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }

	err := sender.Send(context.Background(), Message{
		To:      "maria@example.com",
		Subject: "Código de recuperação",
		Body:    "Olá, Maria.\nSeu código é 123456.",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := <-result
	if got.from != "<nao-responda@soladocerto.local>" || got.to != "<maria@example.com>" {
		t.Fatalf("envelope = %q -> %q", got.from, got.to)
	}

	headers, body, _ := strings.Cut(got.data, "\n\n")
	for _, want := range []string{
		`From: "Solado Certo" <nao-responda@soladocerto.local>`,
		"To: <maria@example.com>",
		"Subject: =?utf-8?q?C=C3=B3digo_de_recupera=C3=A7=C3=A3o?=",
		"Date: Fri, 30 Oct 2026 12:00:00 +0000",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: quoted-printable",
	} {
		if !strings.Contains(headers, want) {
			t.Errorf("headers lack %q:\n%s", want, headers)
		}
	}

	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got := strings.ReplaceAll(string(decoded), "\r\n", "\n"); !strings.HasPrefix(got, "Olá, Maria.\nSeu código é 123456.") {
		t.Fatalf("body = %q", got)
	}
}

func TestSMTPRejectsInvalidAddresses(t *testing.T) {
	tests := map[string]struct {
		from string
		to   string
	}{
		"from": {"not an address", "maria@example.com"},
		"to":   {"nao-responda@soladocerto.local", "not an address"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := NewSMTP("127.0.0.1", "1", "", "", tt.from).Send(context.Background(), Message{To: tt.to})
			if !errors.Is(err, ErrInvalidAddress) {
				t.Fatalf("err = %v, want ErrInvalidAddress", err)
			}
		})
	}
}

func TestSMTPReportsUnreachableServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	listener.Close()

	err = NewSMTP(host, port, "", "", "a@example.com").Send(context.Background(), Message{To: "b@example.com"})
	if err == nil {
		t.Fatal("Send to a closed port succeeded")
	}
}

func TestDisabledDropsMessages(t *testing.T) {
	if err := (Disabled{}).Send(context.Background(), Message{To: "maria@example.com"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
}
