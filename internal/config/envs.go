package config

import (
	"fmt"
	"os"

	"github.com/caslumes/solado_certo_backend/pkg/gateway/asaas"
)

const (
	minSecretKeyLength = 32
	defaultServerPort  = "3000"
	defaultUploadsDir  = "data/uploads"

	PaymentGatewayAsaas = "asaas"
	PaymentGatewayFake  = "fake"
)

type Envs struct {
	ServerPort  string
	DatabaseURL string
	SecretKey   string
	UploadsDir  string

	PaymentGateway    string
	AsaasAPIKey       string
	AsaasBaseURL      string
	AsaasWebhookToken string
}

func LoadEnvs() (*Envs, error) {
	databaseURL, err := getRequiredString("DATABASE_URL")
	if err != nil {
		return nil, err
	}

	secretKey, err := getRequiredString("SECRET_KEY")
	if err != nil {
		return nil, err
	}
	if len(secretKey) < minSecretKeyLength {
		return nil, fmt.Errorf("SECRET_KEY must be at least %d bytes long", minSecretKeyLength)
	}

	paymentGateway := getString("PAYMENT_GATEWAY", PaymentGatewayAsaas)
	if paymentGateway != PaymentGatewayAsaas && paymentGateway != PaymentGatewayFake {
		return nil, fmt.Errorf("PAYMENT_GATEWAY must be %s or %s", PaymentGatewayAsaas, PaymentGatewayFake)
	}
	asaasAPIKey := os.Getenv("ASAAS_API_KEY")
	if paymentGateway == PaymentGatewayAsaas && asaasAPIKey == "" {
		return nil, fmt.Errorf("missing required environment variable ASAAS_API_KEY (or set PAYMENT_GATEWAY=%s to run without Asaas)", PaymentGatewayFake)
	}

	return &Envs{
		ServerPort:        getString("SERVER_PORT", defaultServerPort),
		DatabaseURL:       databaseURL,
		SecretKey:         secretKey,
		UploadsDir:        getString("UPLOADS_DIR", defaultUploadsDir),
		PaymentGateway:    paymentGateway,
		AsaasAPIKey:       asaasAPIKey,
		AsaasBaseURL:      getString("ASAAS_BASE_URL", asaas.SandboxURL),
		AsaasWebhookToken: os.Getenv("ASAAS_WEBHOOK_TOKEN"),
	}, nil
}

func getRequiredString(envVar string) (string, error) {
	value := os.Getenv(envVar)
	if value == "" {
		return "", fmt.Errorf("missing required environment variable %s", envVar)
	}
	return value, nil
}

func getString(envVar string, defaultValue string) string {
	value := os.Getenv(envVar)
	if value != "" {
		return value
	}
	return defaultValue
}
