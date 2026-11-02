package auth

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/pkg/mailer"
)

func (f *fakeRepository) LastPasswordResetAt(_ context.Context, userID string) (*time.Time, error) {
	var last *time.Time
	for _, reset := range f.resets {
		if reset.UserID == userID && (last == nil || reset.CreatedAt.After(*last)) {
			createdAt := reset.CreatedAt
			last = &createdAt
		}
	}
	return last, nil
}

func (f *fakeRepository) CreatePasswordReset(_ context.Context, reset *PasswordResetEntity) error {
	now := time.Now()
	for _, old := range f.resets {
		if old.UserID == reset.UserID && old.ExpiresAt.After(now) {
			old.ExpiresAt = now
		}
	}
	reset.ID = "reset-" + string(rune('a'+len(f.resets)))
	reset.CreatedAt = now
	f.resets = append(f.resets, reset)
	return nil
}

func (f *fakeRepository) activeReset(userID string) *PasswordResetEntity {
	var latest *PasswordResetEntity
	for _, reset := range f.resets {
		if reset.UserID == userID && reset.ExpiresAt.After(time.Now()) && (latest == nil || !reset.CreatedAt.Before(latest.CreatedAt)) {
			latest = reset
		}
	}
	return latest
}

func (f *fakeRepository) ClaimPasswordResetAttempt(_ context.Context, userID string, maxAttempts int) (*PasswordResetEntity, error) {
	reset := f.activeReset(userID)
	if reset == nil {
		return nil, app.ErrInvalidResetCode
	}
	if reset.Attempts >= maxAttempts {
		return nil, app.ErrResetAttemptsExceeded
	}
	reset.Attempts++
	return reset, nil
}

func (f *fakeRepository) CompletePasswordReset(_ context.Context, resetID string, userID string, passwordHash string) error {
	reset := f.activeReset(userID)
	if reset == nil || reset.ID != resetID {
		return app.ErrInvalidResetCode
	}
	reset.ExpiresAt = time.Now()
	for _, ue := range f.users {
		if ue.ID == userID {
			ue.PasswordHash = passwordHash
		}
	}
	return f.RevokeUserTokens(context.Background(), userID)
}

type fakeMailer struct {
	mu   sync.Mutex
	sent []mailer.Message
	err  error
}

func (m *fakeMailer) Send(_ context.Context, message mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, message)
	return m.err
}

var sentCodePattern = regexp.MustCompile(`código (\d{6})`)

func (m *fakeMailer) lastCode(t *testing.T) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.sent) == 0 {
		t.Fatal("no e-mail was sent")
	}
	match := sentCodePattern.FindStringSubmatch(m.sent[len(m.sent)-1].Body)
	if match == nil {
		t.Fatalf("no code in the e-mail: %q", m.sent[len(m.sent)-1].Body)
	}
	return match[1]
}

const resetEmail = "ana@example.com"

func newPasswordService(t *testing.T) (*service, *fakeRepository, *fakeMailer) {
	t.Helper()

	repository := newFakeRepository()
	s := NewService(repository, testSecretKey)
	mail := &fakeMailer{}
	s.SetMailer(mail)

	err := s.Register(context.Background(), &RegisterInput{
		Name: "Ana", Email: resetEmail, Phone: "19912345678", Password: "old-password",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	return s, repository, mail
}

func requestCode(t *testing.T, s *service, mail *fakeMailer) string {
	t.Helper()

	if err := s.ForgotPassword(context.Background(), resetEmail); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	s.WaitForMail()

	return mail.lastCode(t)
}

func wrongCode(code string) string {
	if code == "000000" {
		return "000001"
	}
	return "000000"
}

func TestForgotPasswordSendsCodeToUser(t *testing.T) {
	s, repository, mail := newPasswordService(t)

	if err := s.ForgotPassword(context.Background(), "  ANA@example.com "); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	s.WaitForMail()

	if len(mail.sent) != 1 || mail.sent[0].To != resetEmail || !strings.Contains(mail.sent[0].Body, "Olá, Ana.") {
		t.Fatalf("sent = %+v", mail.sent)
	}
	code := mail.lastCode(t)
	if len(repository.resets) != 1 {
		t.Fatalf("resets = %d, want 1", len(repository.resets))
	}
	reset := repository.resets[0]
	if strings.Contains(reset.CodeHash, code) {
		t.Fatalf("the stored hash %q contains the code", reset.CodeHash)
	}
	if lifetime := time.Until(reset.ExpiresAt); lifetime < ResetCodeLifetime-time.Minute || lifetime > ResetCodeLifetime {
		t.Fatalf("code lasts %v, want %v", lifetime, ResetCodeLifetime)
	}
}

func TestForgotPasswordAnswersTheSameForUnknownEmail(t *testing.T) {
	s, repository, mail := newPasswordService(t)

	if err := s.ForgotPassword(context.Background(), "nobody@example.com"); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	s.WaitForMail()

	if len(mail.sent) != 0 || len(repository.resets) != 0 {
		t.Fatalf("sent = %+v, resets = %d", mail.sent, len(repository.resets))
	}
}

func TestForgotPasswordRejectsInvalidEmail(t *testing.T) {
	s, _, _ := newPasswordService(t)

	if err := s.ForgotPassword(context.Background(), "ana"); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("err = %v, want %v", err, app.ErrInvalidInput)
	}
}

func TestForgotPasswordWaitsBeforeSendingAnotherCode(t *testing.T) {
	s, repository, mail := newPasswordService(t)
	first := requestCode(t, s, mail)

	if err := s.ForgotPassword(context.Background(), resetEmail); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	s.WaitForMail()
	if len(mail.sent) != 1 || len(repository.resets) != 1 {
		t.Fatalf("a second code was sent within %v", ResetResendInterval)
	}

	s.now = func() time.Time { return time.Now().Add(ResetResendInterval) }
	second := requestCode(t, s, mail)
	if len(mail.sent) != 2 {
		t.Fatalf("sent = %d, want 2", len(mail.sent))
	}

	err := s.ResetPassword(context.Background(), &ResetPasswordInput{Email: resetEmail, Code: first, NewPassword: "new-password"})
	if first != second && !errors.Is(err, app.ErrInvalidResetCode) {
		t.Fatalf("the replaced code: err = %v, want %v", err, app.ErrInvalidResetCode)
	}
}

func TestForgotPasswordSucceedsWhenMailFails(t *testing.T) {
	s, repository, mail := newPasswordService(t)
	mail.err = errors.New("smtp down")

	if err := s.ForgotPassword(context.Background(), resetEmail); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	s.WaitForMail()

	if len(repository.resets) != 1 {
		t.Fatalf("resets = %d, want 1", len(repository.resets))
	}
}

func TestResetPasswordChangesPasswordAndEndsSessions(t *testing.T) {
	s, _, mail := newPasswordService(t)
	tokens, err := s.Login(context.Background(), &LoginInput{Email: resetEmail, Password: "old-password"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	code := requestCode(t, s, mail)

	err = s.ResetPassword(context.Background(), &ResetPasswordInput{Email: " Ana@Example.com", Code: code, NewPassword: "new-password"})
	if err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	if _, err := s.Login(context.Background(), &LoginInput{Email: resetEmail, Password: "old-password"}); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("old password: err = %v, want %v", err, app.ErrInvalidCredentials)
	}
	if _, err := s.Login(context.Background(), &LoginInput{Email: resetEmail, Password: "new-password"}); err != nil {
		t.Fatalf("new password: %v", err)
	}
	if _, err := s.RefreshToken(context.Background(), tokens.RefreshToken); !errors.Is(err, app.ErrTokenRevoked) {
		t.Fatalf("old session: err = %v, want %v", err, app.ErrTokenRevoked)
	}
}

func TestResetPasswordCodeWorksOnce(t *testing.T) {
	s, _, mail := newPasswordService(t)
	code := requestCode(t, s, mail)
	input := &ResetPasswordInput{Email: resetEmail, Code: code, NewPassword: "new-password"}

	if err := s.ResetPassword(context.Background(), input); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if err := s.ResetPassword(context.Background(), input); !errors.Is(err, app.ErrInvalidResetCode) {
		t.Fatalf("reuse: err = %v, want %v", err, app.ErrInvalidResetCode)
	}
}

func TestResetPasswordRejectsExpiredCode(t *testing.T) {
	s, repository, mail := newPasswordService(t)
	code := requestCode(t, s, mail)
	repository.resets[0].ExpiresAt = time.Now().Add(-time.Second)

	err := s.ResetPassword(context.Background(), &ResetPasswordInput{Email: resetEmail, Code: code, NewPassword: "new-password"})
	if !errors.Is(err, app.ErrInvalidResetCode) {
		t.Fatalf("err = %v, want %v", err, app.ErrInvalidResetCode)
	}
}

func TestResetPasswordLocksCodeAfterTooManyAttempts(t *testing.T) {
	s, _, mail := newPasswordService(t)
	code := requestCode(t, s, mail)

	for attempt := 1; attempt <= MaxResetAttempts; attempt++ {
		err := s.ResetPassword(context.Background(), &ResetPasswordInput{Email: resetEmail, Code: wrongCode(code), NewPassword: "new-password"})
		if !errors.Is(err, app.ErrInvalidResetCode) {
			t.Fatalf("attempt %d: err = %v, want %v", attempt, err, app.ErrInvalidResetCode)
		}
	}

	err := s.ResetPassword(context.Background(), &ResetPasswordInput{Email: resetEmail, Code: code, NewPassword: "new-password"})
	if !errors.Is(err, app.ErrResetAttemptsExceeded) {
		t.Fatalf("right code after the limit: err = %v, want %v", err, app.ErrResetAttemptsExceeded)
	}
}

func TestResetPasswordRejectsInvalidInput(t *testing.T) {
	tests := map[string]struct {
		input ResetPasswordInput
		want  error
	}{
		"unknown e-mail": {ResetPasswordInput{Email: "nobody@example.com", Code: "123456", NewPassword: "new-password"}, app.ErrInvalidResetCode},
		"invalid e-mail": {ResetPasswordInput{Email: "ana", Code: "123456", NewPassword: "new-password"}, app.ErrInvalidInput},
		"short password": {ResetPasswordInput{Email: resetEmail, Code: "123456", NewPassword: "short"}, app.ErrInvalidInput},
		"letters":        {ResetPasswordInput{Email: resetEmail, Code: "12a456", NewPassword: "new-password"}, app.ErrInvalidResetCode},
		"five digits":    {ResetPasswordInput{Email: resetEmail, Code: "12345", NewPassword: "new-password"}, app.ErrInvalidResetCode},
		"no code sent":   {ResetPasswordInput{Email: resetEmail, Code: "123456", NewPassword: "new-password"}, app.ErrInvalidResetCode},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s, _, _ := newPasswordService(t)

			if err := s.ResetPassword(context.Background(), &tt.input); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}

	s, _, _ := newPasswordService(t)
	if err := s.ResetPassword(context.Background(), nil); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("nil input: err = %v, want %v", err, app.ErrInvalidInput)
	}
}

func TestResetCodeHashIsSaltedAndKeyed(t *testing.T) {
	s := NewService(newFakeRepository(), testSecretKey)

	first, err := s.hashResetCode("user-1", "123456")
	if err != nil {
		t.Fatalf("hashResetCode: %v", err)
	}
	second, err := s.hashResetCode("user-1", "123456")
	if err != nil {
		t.Fatalf("hashResetCode: %v", err)
	}
	if first == second {
		t.Fatal("the same code gave the same hash twice")
	}

	if !s.resetCodeMatches(first, "user-1", "123456") {
		t.Fatal("the code does not match its hash")
	}
	if s.resetCodeMatches(first, "user-2", "123456") {
		t.Fatal("the hash matches another user")
	}
	if s.resetCodeMatches(first, "user-1", "654321") {
		t.Fatal("the hash matches another code")
	}
	if NewService(newFakeRepository(), "other-secret-key").resetCodeMatches(first, "user-1", "123456") {
		t.Fatal("the hash matches under another secret key")
	}
	for _, malformed := range []string{"", "no-dot", "zz.00", "00.zz"} {
		if s.resetCodeMatches(malformed, "user-1", "123456") {
			t.Fatalf("malformed hash %q matched", malformed)
		}
	}
}

func TestNewResetCodeHasSixDigits(t *testing.T) {
	for range 50 {
		code, err := newResetCode()
		if err != nil {
			t.Fatalf("newResetCode: %v", err)
		}
		if !resetCodePattern.MatchString(code) {
			t.Fatalf("code = %q", code)
		}
	}
}
