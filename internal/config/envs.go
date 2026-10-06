package config

import (
	"fmt"
	"os"
	"strconv"
)

const minSecretKeyLength = 32

type Envs struct {
	ServerPort  string `env:"SERVER_PORT" envDefault:"8080"`
	DatabaseURL string `env:"DATABASE_URL,required"`
	SecretKey   string `env:"SECRET_KEY,required"`
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
		ServerPort:  getString("SERVER_PORT", "8080"),
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

func getInt64(envVar string, defaultValue int64) int64 {
	value := os.Getenv(envVar)
	if value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}
