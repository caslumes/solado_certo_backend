package config

import (
	"fmt"
	"os"
)

const (
	minSecretKeyLength = 32
	defaultServerPort  = "3000"
)

type Envs struct {
	ServerPort  string
	DatabaseURL string
	SecretKey   string
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

	return &Envs{
		ServerPort:  getString("SERVER_PORT", defaultServerPort),
		DatabaseURL: databaseURL,
		SecretKey:   secretKey,
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
