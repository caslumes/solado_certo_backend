package config

import (
	"os"
	"strconv"
)

type Envs struct {
	ServerPort  string `env:"SERVER_PORT" envDefault:"8080"`
	DatabaseURL string `env:"DATABASE_URL"`
	SecretKey   string `env:"SECRET_KEY" envDefault:"secret_key"`
}

func LoadEnvs() *Envs {
	return &Envs{
		ServerPort:  getString("SERVER_PORT", "8080"),
		DatabaseURL: getString("DATABASE_URL", "postgres://user:password@localhost:5432/dbname"),
		SecretKey:   getString("SECRET_KEY", "secret_key"),
	}
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
