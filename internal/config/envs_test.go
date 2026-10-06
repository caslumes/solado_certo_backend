package config

import (
	"strings"
	"testing"
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
			if envs.SecretKey != tt.secretKey || envs.DatabaseURL != tt.databaseURL || envs.ServerPort != defaultServerPort {
				t.Fatalf("envs = %+v", envs)
			}
		})
	}
}
