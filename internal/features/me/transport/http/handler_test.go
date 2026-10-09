package httpme

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

const testUserID = "user-1"

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeService struct {
	Service
	editPodologicalProfile func(context.Context, string, me.PodologicalProfileInput) (*me.PodologicalProfileEntity, error)
	getPainPoints          func(context.Context) ([]*me.PainPointEntity, error)
	grantConsent           func(context.Context, string, me.ConsentInput) (*me.ConsentEntity, bool, error)
	revokeConsent          func(context.Context, string, string) error
	changeOnboardingStep   func(context.Context, string, me.OnboardingInput) error
}

func (f fakeService) EditPodologicalProfile(ctx context.Context, userID string, input me.PodologicalProfileInput) (*me.PodologicalProfileEntity, error) {
	return f.editPodologicalProfile(ctx, userID, input)
}

func (f fakeService) GetPainPoints(ctx context.Context) ([]*me.PainPointEntity, error) {
	return f.getPainPoints(ctx)
}

func (f fakeService) GrantConsent(ctx context.Context, userID string, input me.ConsentInput) (*me.ConsentEntity, bool, error) {
	return f.grantConsent(ctx, userID, input)
}

func (f fakeService) RevokeConsent(ctx context.Context, userID string, purpose string) error {
	return f.revokeConsent(ctx, userID, purpose)
}

func (f fakeService) ChangeOnboardingStep(ctx context.Context, userID string, input me.OnboardingInput) error {
	return f.changeOnboardingStep(ctx, userID, input)
}

func serve(handler *Handler, request *http.Request) *httptest.ResponseRecorder {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(authmiddleware.WithUserID(c.Request.Context(), testUserID))
	})
	router.PUT("/me/podological-profile", handler.EditPodologicalProfile)
	router.GET("/me/pain-points", handler.GetPainPoints)
	router.POST("/me/consents", handler.GrantConsent)
	router.DELETE("/me/consents/:purpose", handler.RevokeConsent)
	router.PUT("/me/onboarding", handler.ChangeOnboardingStep)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

const podologicalProfileBody = `{"footstrike_type":"neutral","clinical_condition":null,"pain_point_ids":["00000000-0000-0000-0000-000000000701"]}`

func TestEditPodologicalProfileDecodesRequest(t *testing.T) {
	var received me.PodologicalProfileInput
	handler := NewHandler(fakeService{
		editPodologicalProfile: func(_ context.Context, userID string, input me.PodologicalProfileInput) (*me.PodologicalProfileEntity, error) {
			if userID != testUserID {
				t.Fatalf("userID = %q", userID)
			}
			received = input
			return &me.PodologicalProfileEntity{FootstrikeType: input.FootstrikeType, PainPoints: []me.PainPointEntity{}}, nil
		},
	})

	recorder := serve(handler, httptest.NewRequest(http.MethodPut, "/me/podological-profile", strings.NewReader(podologicalProfileBody)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if received.FootstrikeType != "neutral" || received.ClinicalCondition != nil || len(received.PainPointIDs) != 1 {
		t.Fatalf("input = %+v", received)
	}
	if !strings.Contains(recorder.Body.String(), `"pain_points":[]`) {
		t.Fatalf("body = %s, want an empty pain_points list", recorder.Body.String())
	}
}

func TestEditPodologicalProfileMapsErrors(t *testing.T) {
	tests := map[string]struct {
		err  error
		want int
	}{
		"no consent":    {app.ErrConsentRequired, http.StatusForbidden},
		"invalid input": {app.ErrInvalidInput, http.StatusBadRequest},
		"unexpected":    {errors.New("database unavailable"), http.StatusInternalServerError},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewHandler(fakeService{
				editPodologicalProfile: func(context.Context, string, me.PodologicalProfileInput) (*me.PodologicalProfileEntity, error) {
					return nil, test.err
				},
			})

			recorder := serve(handler, httptest.NewRequest(http.MethodPut, "/me/podological-profile", strings.NewReader(podologicalProfileBody)))

			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}

func TestGetPainPointsReturnsList(t *testing.T) {
	handler := NewHandler(fakeService{
		getPainPoints: func(context.Context) ([]*me.PainPointEntity, error) {
			return []*me.PainPointEntity{{ID: "00000000-0000-0000-0000-000000000701", Name: "Calcanhar"}}, nil
		},
	})

	recorder := serve(handler, httptest.NewRequest(http.MethodGet, "/me/pain-points", nil))

	var body []me.PainPointOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if recorder.Code != http.StatusOK || len(body) != 1 || body[0].Name != "Calcanhar" {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestGrantConsentReturnsCreatedOrOK(t *testing.T) {
	tests := map[string]struct {
		created bool
		want    int
	}{
		"new consent":     {true, http.StatusCreated},
		"already granted": {false, http.StatusOK},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var received me.ConsentInput
			handler := NewHandler(fakeService{
				grantConsent: func(_ context.Context, _ string, input me.ConsentInput) (*me.ConsentEntity, bool, error) {
					received = input
					return &me.ConsentEntity{Purpose: me.ConsentPurpose(input.Purpose), TermsVersion: input.TermsVersion, GrantedAt: time.Now()}, test.created, nil
				},
			})

			body := `{"purpose":"podological_profile","terms_version":"podological-profile-v1"}`
			recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/me/consents", strings.NewReader(body)))

			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
			if received.Purpose != "podological_profile" || received.TermsVersion != "podological-profile-v1" {
				t.Fatalf("input = %+v", received)
			}
		})
	}
}

func TestGrantConsentMapsInvalidInput(t *testing.T) {
	handler := NewHandler(fakeService{
		grantConsent: func(context.Context, string, me.ConsentInput) (*me.ConsentEntity, bool, error) {
			return nil, false, app.ErrInvalidInput
		},
	})

	recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/me/consents", strings.NewReader(`{"purpose":"marketing"}`)))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestRevokeConsentPassesPurpose(t *testing.T) {
	var received string
	handler := NewHandler(fakeService{
		revokeConsent: func(_ context.Context, _ string, purpose string) error {
			received = purpose
			return nil
		},
	})

	recorder := serve(handler, httptest.NewRequest(http.MethodDelete, "/me/consents/podological_profile", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if received != "podological_profile" {
		t.Fatalf("purpose = %q", received)
	}
}

func TestRevokeConsentMapsInvalidInput(t *testing.T) {
	handler := NewHandler(fakeService{
		revokeConsent: func(context.Context, string, string) error {
			return app.ErrInvalidInput
		},
	})

	recorder := serve(handler, httptest.NewRequest(http.MethodDelete, "/me/consents/marketing", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestChangeOnboardingStepMapsErrors(t *testing.T) {
	tests := map[string]struct {
		err  error
		want int
	}{
		"changed":            {nil, http.StatusNoContent},
		"invalid transition": {app.ErrInvalidInput, http.StatusBadRequest},
		"already completed":  {app.ErrOnboardingAlreadyCompleted, http.StatusBadRequest},
		"unknown step":       {app.ErrOnboardingStepNotFound, http.StatusNotFound},
		"no onboarding":      {app.ErrOnboardingNotFound, http.StatusNotFound},
		"unexpected":         {errors.New("database unavailable"), http.StatusInternalServerError},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewHandler(fakeService{
				changeOnboardingStep: func(context.Context, string, me.OnboardingInput) error {
					return test.err
				},
			})

			body := `{"step":"welcome","action":"skip"}`
			recorder := serve(handler, httptest.NewRequest(http.MethodPut, "/me/onboarding", strings.NewReader(body)))

			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}
