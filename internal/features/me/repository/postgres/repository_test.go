package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
)

func TestGetProfileReturnsNotFoundForUnknownUser(t *testing.T) {
	r := NewPostgresRepository(testutil.DB(t))

	_, err := r.GetProfileByUserID(context.Background(), "00000000-0000-0000-0000-000000000000")

	if !errors.Is(err, app.ErrUserNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrUserNotFound)
	}
}

func TestUpdateProfileRejectsEmailOrPhoneInUse(t *testing.T) {
	pool := testutil.DB(t)
	r := NewPostgresRepository(pool)
	existing := testutil.InsertUser(t, pool)
	user := testutil.InsertUser(t, pool)

	tests := map[string]me.ProfileEntity{
		"e-mail": {ID: user.ID, Name: user.Name, Email: existing.Email, Phone: user.Phone, Status: enum.ProfileStatusActive},
		"phone":  {ID: user.ID, Name: user.Name, Email: user.Email, Phone: existing.Phone, Status: enum.ProfileStatusActive},
	}

	for name, profile := range tests {
		t.Run(name, func(t *testing.T) {
			err := r.UpdateProfile(context.Background(), &profile)
			if !errors.Is(err, app.ErrUserAlreadyExists) {
				t.Fatalf("err = %v, want %v", err, app.ErrUserAlreadyExists)
			}
		})
	}
}
