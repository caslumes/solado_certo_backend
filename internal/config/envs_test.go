package config

import (
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/pkg/gateway/asaas"
	"github.com/caslumes/solado_certo_backend/pkg/mailer"
)

const validSecretKey = "0123456789abcdef0123456789abcdef"

func TestLoadEnvs(t *testing.T) {
	tests := []struct {
		name        string
		databaseURL string
		secretKey   string
		wantErr     string
	}{
		{"valid", "postgres://localhost/db", validSecretKey, ""},
		{"missing secret key", "postgres://localhost/db", "", "SECRET_KEY"},
		{"short secret key", "postgres://localhost/db", "secret_key", "at least 32 bytes"},
		{"missing database URL", "", validSecretKey, "DATABASE_URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SERVER_PORT", "")
			t.Setenv("UPLOADS_DIR", "")
			t.Setenv("PAYMENT_GATEWAY", "")
			t.Setenv("ASAAS_API_KEY", "asaas-key")
			t.Setenv("DATABASE_URL", tt.databaseURL)
			t.Setenv("SECRET_KEY", tt.secretKey)

			envs, err := LoadEnvs()

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadEnvs: %v", err)
			}
			if envs.SecretKey != tt.secretKey || envs.DatabaseURL != tt.databaseURL || envs.ServerPort != defaultServerPort || envs.UploadsDir != defaultUploadsDir {
				t.Fatalf("envs = %+v", envs)
			}
		})
	}
}

func TestLoadEnvsPaymentGateway(t *testing.T) {
	tests := []struct {
		name        string
		gateway     string
		apiKey      string
		wantGateway string
		wantErr     string
	}{
		{"asaas by default", "", "asaas-key", PaymentGatewayAsaas, ""},
		{"asaas without a key", PaymentGatewayAsaas, "", "", "ASAAS_API_KEY"},
		{"fake without a key", PaymentGatewayFake, "", PaymentGatewayFake, ""},
		{"unknown gateway", "stripe", "asaas-key", "", "PAYMENT_GATEWAY must be"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://localhost/db")
			t.Setenv("SECRET_KEY", validSecretKey)
			t.Setenv("PAYMENT_GATEWAY", tt.gateway)
			t.Setenv("ASAAS_API_KEY", tt.apiKey)
			t.Setenv("ASAAS_BASE_URL", "")
			t.Setenv("ASAAS_WEBHOOK_TOKEN", "webhook-token")

			envs, err := LoadEnvs()

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadEnvs: %v", err)
			}
			if envs.PaymentGateway != tt.wantGateway || envs.AsaasBaseURL != asaas.SandboxURL || envs.AsaasWebhookToken != "webhook-token" {
				t.Fatalf("envs = %+v", envs)
			}
		})
	}
}

func TestNewPaymentGateway(t *testing.T) {
	for _, name := range []string{PaymentGatewayAsaas, PaymentGatewayFake} {
		got := newPaymentGateway(&Envs{PaymentGateway: name, AsaasBaseURL: asaas.SandboxURL, AsaasAPIKey: "key"})
		if got.Name() != name {
			t.Errorf("PAYMENT_GATEWAY=%s gives %q", name, got.Name())
		}
	}
}

func TestLoadEnvsMail(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("SECRET_KEY", validSecretKey)
	t.Setenv("PAYMENT_GATEWAY", PaymentGatewayFake)
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_PORT", "")
	t.Setenv("MAIL_FROM", "")

	envs, err := LoadEnvs()
	if err != nil {
		t.Fatalf("LoadEnvs: %v", err)
	}
	if envs.SMTPHost != "" || envs.SMTPPort != defaultSMTPPort || envs.MailFrom != defaultMailFrom {
		t.Fatalf("envs = %+v", envs)
	}

	t.Setenv("SMTP_HOST", "mailpit")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("MAIL_FROM", "Loja <loja@example.com>")
	envs, err = LoadEnvs()
	if err != nil {
		t.Fatalf("LoadEnvs: %v", err)
	}
	if envs.SMTPHost != "mailpit" || envs.SMTPPort != "2525" || envs.MailFrom != "Loja <loja@example.com>" {
		t.Fatalf("envs = %+v", envs)
	}
}

func TestNewMailer(t *testing.T) {
	if _, ok := newMailer(&Envs{}).(mailer.Disabled); !ok {
		t.Error("no SMTP_HOST should disable e-mail")
	}
	if _, ok := newMailer(&Envs{SMTPHost: "mailpit", SMTPPort: "1025"}).(*mailer.SMTP); !ok {
		t.Error("SMTP_HOST should send through SMTP")
	}
}
