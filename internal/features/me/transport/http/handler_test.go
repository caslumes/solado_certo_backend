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
	getProfile             func(context.Context, string) (*me.ProfileEntity, error)
	editProfile            func(context.Context, string, me.ProfileInput) (*me.ProfileEntity, error)
	getAddresses           func(context.Context, string) ([]*me.AddressEntity, error)
	addAddress             func(context.Context, string, me.AddressInput) (*me.AddressEntity, error)
	editAddress            func(context.Context, string, string, me.AddressInput) (*me.AddressEntity, error)
	deleteAddress          func(context.Context, string, string) error
	getPodologicalProfile  func(context.Context, string) (*me.PodologicalProfileEntity, error)
	editPodologicalProfile func(context.Context, string, me.PodologicalProfileInput) (*me.PodologicalProfileEntity, error)
	getPainPoints          func(context.Context) ([]*me.PainPointEntity, error)
	getConsents            func(context.Context, string) ([]*me.ConsentEntity, error)
	grantConsent           func(context.Context, string, me.ConsentInput) (*me.ConsentEntity, bool, error)
	revokeConsent          func(context.Context, string, string) error
	getOnboarding          func(context.Context, string) (*me.OnboardingEntity, error)
	changeOnboardingStep   func(context.Context, string, me.OnboardingInput) error
}

func (f fakeService) GetProfileByUserID(ctx context.Context, userID string) (*me.ProfileEntity, error) {
	return f.getProfile(ctx, userID)
}

func (f fakeService) EditProfile(ctx context.Context, userID string, input me.ProfileInput) (*me.ProfileEntity, error) {
	return f.editProfile(ctx, userID, input)
}

func (f fakeService) GetAddressesByUserID(ctx context.Context, userID string) ([]*me.AddressEntity, error) {
	return f.getAddresses(ctx, userID)
}

func (f fakeService) AddAddress(ctx context.Context, userID string, input me.AddressInput) (*me.AddressEntity, error) {
	return f.addAddress(ctx, userID, input)
}

func (f fakeService) EditAddress(ctx context.Context, userID string, addressID string, input me.AddressInput) (*me.AddressEntity, error) {
	return f.editAddress(ctx, userID, addressID, input)
}

func (f fakeService) DeleteAddress(ctx context.Context, userID string, addressID string) error {
	return f.deleteAddress(ctx, userID, addressID)
}

func (f fakeService) GetPodologicalProfileByUserID(ctx context.Context, userID string) (*me.PodologicalProfileEntity, error) {
	return f.getPodologicalProfile(ctx, userID)
}

func (f fakeService) EditPodologicalProfile(ctx context.Context, userID string, input me.PodologicalProfileInput) (*me.PodologicalProfileEntity, error) {
	return f.editPodologicalProfile(ctx, userID, input)
}

func (f fakeService) GetPainPoints(ctx context.Context) ([]*me.PainPointEntity, error) {
	return f.getPainPoints(ctx)
}

func (f fakeService) GetConsents(ctx context.Context, userID string) ([]*me.ConsentEntity, error) {
	return f.getConsents(ctx, userID)
}

func (f fakeService) GrantConsent(ctx context.Context, userID string, input me.ConsentInput) (*me.ConsentEntity, bool, error) {
	return f.grantConsent(ctx, userID, input)
}

func (f fakeService) RevokeConsent(ctx context.Context, userID string, purpose string) error {
	return f.revokeConsent(ctx, userID, purpose)
}

func (f fakeService) GetOnboardingByUserID(ctx context.Context, userID string) (*me.OnboardingEntity, error) {
	return f.getOnboarding(ctx, userID)
}

func (f fakeService) ChangeOnboardingStep(ctx context.Context, userID string, input me.OnboardingInput) error {
	return f.changeOnboardingStep(ctx, userID, input)
}

func newRouter(handler *Handler, authenticated bool) *gin.Engine {
	router := gin.New()
	if authenticated {
		router.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(authmiddleware.WithUserID(c.Request.Context(), testUserID))
		})
	}
	router.GET("/me/profile", handler.GetProfile)
	router.PATCH("/me/profile", handler.EditProfile)
	router.GET("/me/addresses", handler.GetAddresses)
	router.POST("/me/addresses", handler.AddAddress)
	router.PATCH("/me/addresses/:addressID", handler.EditAddress)
	router.DELETE("/me/addresses/:addressID", handler.DeleteAddress)
	router.GET("/me/podological-profile", handler.GetPodologicalProfile)
	router.PUT("/me/podological-profile", handler.EditPodologicalProfile)
	router.GET("/me/pain-points", handler.GetPainPoints)
	router.GET("/me/consents", handler.GetConsents)
	router.POST("/me/consents", handler.GrantConsent)
	router.DELETE("/me/consents/:purpose", handler.RevokeConsent)
	router.GET("/me/onboarding", handler.GetOnboarding)
	router.PUT("/me/onboarding", handler.ChangeOnboardingStep)
	return router
}

func serve(handler *Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	newRouter(handler, true).ServeHTTP(recorder, request)
	return recorder
}

func serveAnonymous(handler *Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	newRouter(handler, false).ServeHTTP(recorder, request)
	return recorder
}

func decode[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var body T
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
	return body
}

func request(method, target, body string) *http.Request {
	if body == "" {
		return httptest.NewRequest(method, target, nil)
	}
	return httptest.NewRequest(method, target, strings.NewReader(body))
}

var errUnexpected = errors.New("database unavailable")

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

type endpoint struct {
	method, target, body string
}

var userEndpoints = map[string]endpoint{
	"get profile":              {http.MethodGet, "/me/profile", ""},
	"edit profile":             {http.MethodPatch, "/me/profile", `{"name":"Maria"}`},
	"get addresses":            {http.MethodGet, "/me/addresses", ""},
	"add address":              {http.MethodPost, "/me/addresses", `{"street":"Rua A"}`},
	"edit address":             {http.MethodPatch, "/me/addresses/address-1", `{"street":"Rua A"}`},
	"delete address":           {http.MethodDelete, "/me/addresses/address-1", ""},
	"get podological profile":  {http.MethodGet, "/me/podological-profile", ""},
	"edit podological profile": {http.MethodPut, "/me/podological-profile", podologicalProfileBody},
	"get consents":             {http.MethodGet, "/me/consents", ""},
	"grant consent":            {http.MethodPost, "/me/consents", `{"purpose":"podological_profile"}`},
	"revoke consent":           {http.MethodDelete, "/me/consents/podological_profile", ""},
	"get onboarding":           {http.MethodGet, "/me/onboarding", ""},
	"change onboarding step":   {http.MethodPut, "/me/onboarding", `{"step":"welcome","action":"advance"}`},
}

func TestEndpointsRequireAuthenticatedUser(t *testing.T) {
	for name, e := range userEndpoints {
		t.Run(name, func(t *testing.T) {
			recorder := serveAnonymous(NewHandler(fakeService{}), request(e.method, e.target, e.body))

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestEndpointsRejectMalformedJSON(t *testing.T) {
	for name, e := range userEndpoints {
		if e.body == "" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			recorder := serve(NewHandler(fakeService{}), request(e.method, e.target, `{"name":`))

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}

func failingService() fakeService {
	return fakeService{
		getProfile:   func(context.Context, string) (*me.ProfileEntity, error) { return nil, errUnexpected },
		editProfile:  func(context.Context, string, me.ProfileInput) (*me.ProfileEntity, error) { return nil, errUnexpected },
		getAddresses: func(context.Context, string) ([]*me.AddressEntity, error) { return nil, errUnexpected },
		addAddress:   func(context.Context, string, me.AddressInput) (*me.AddressEntity, error) { return nil, errUnexpected },
		editAddress: func(context.Context, string, string, me.AddressInput) (*me.AddressEntity, error) {
			return nil, errUnexpected
		},
		deleteAddress: func(context.Context, string, string) error { return errUnexpected },
		getPodologicalProfile: func(context.Context, string) (*me.PodologicalProfileEntity, error) {
			return nil, errUnexpected
		},
		editPodologicalProfile: func(context.Context, string, me.PodologicalProfileInput) (*me.PodologicalProfileEntity, error) {
			return nil, errUnexpected
		},
		getPainPoints: func(context.Context) ([]*me.PainPointEntity, error) { return nil, errUnexpected },
		getConsents:   func(context.Context, string) ([]*me.ConsentEntity, error) { return nil, errUnexpected },
		grantConsent: func(context.Context, string, me.ConsentInput) (*me.ConsentEntity, bool, error) {
			return nil, false, errUnexpected
		},
		revokeConsent:        func(context.Context, string, string) error { return errUnexpected },
		getOnboarding:        func(context.Context, string) (*me.OnboardingEntity, error) { return nil, errUnexpected },
		changeOnboardingStep: func(context.Context, string, me.OnboardingInput) error { return errUnexpected },
	}
}

func TestEndpointsHideUnexpectedErrors(t *testing.T) {
	endpoints := map[string]endpoint{"get pain points": {http.MethodGet, "/me/pain-points", ""}}
	for name, e := range userEndpoints {
		endpoints[name] = e
	}

	for name, e := range endpoints {
		t.Run(name, func(t *testing.T) {
			recorder := serve(NewHandler(failingService()), request(e.method, e.target, e.body))

			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
			}
			if strings.Contains(recorder.Body.String(), errUnexpected.Error()) {
				t.Fatalf("body leaks the internal error: %s", recorder.Body.String())
			}
		})
	}
}

func TestGetProfileReturnsProfile(t *testing.T) {
	handler := NewHandler(fakeService{
		getProfile: func(_ context.Context, userID string) (*me.ProfileEntity, error) {
			if userID != testUserID {
				t.Fatalf("userID = %q", userID)
			}
			return &me.ProfileEntity{ID: testUserID, Name: "Maria", Email: "maria@example.com", Phone: "11912345678"}, nil
		},
	})

	recorder := serve(handler, request(http.MethodGet, "/me/profile", ""))

	want := `{"name":"Maria","email":"maria@example.com","phone":"11912345678","avatar_url":""}`
	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != want {
		t.Fatalf("status = %d, body = %s, want %s", recorder.Code, recorder.Body.String(), want)
	}
}

func TestEditProfileDecodesRequest(t *testing.T) {
	var received me.ProfileInput
	handler := NewHandler(fakeService{
		editProfile: func(_ context.Context, _ string, input me.ProfileInput) (*me.ProfileEntity, error) {
			received = input
			return &me.ProfileEntity{Name: input.Name, Email: input.Email, Phone: input.Phone}, nil
		},
	})

	body := `{"name":"Maria","email":"maria@example.com","phone":"11912345678","avatar_url":"https://example.com/a.png"}`
	recorder := serve(handler, request(http.MethodPatch, "/me/profile", body))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	want := me.ProfileInput{Name: "Maria", Email: "maria@example.com", Phone: "11912345678", AvatarURL: "https://example.com/a.png"}
	if received != want {
		t.Fatalf("input = %+v, want %+v", received, want)
	}
}

func TestEditProfileMapsErrors(t *testing.T) {
	tests := map[string]struct {
		err  error
		want int
	}{
		"invalid input":          {app.ErrInvalidInput, http.StatusBadRequest},
		"e-mail or phone in use": {app.ErrUserAlreadyExists, http.StatusConflict},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewHandler(fakeService{
				editProfile: func(context.Context, string, me.ProfileInput) (*me.ProfileEntity, error) {
					return nil, test.err
				},
			})

			recorder := serve(handler, request(http.MethodPatch, "/me/profile", `{"email":"maria@example.com"}`))

			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}

func TestGetAddressesReturnsEmptyList(t *testing.T) {
	handler := NewHandler(fakeService{
		getAddresses: func(context.Context, string) ([]*me.AddressEntity, error) { return nil, nil },
	})

	recorder := serve(handler, request(http.MethodGet, "/me/addresses", ""))

	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != "[]" {
		t.Fatalf("status = %d, body = %s, want 200 and []", recorder.Code, recorder.Body.String())
	}
}

func TestGetAddressesReturnsList(t *testing.T) {
	handler := NewHandler(fakeService{
		getAddresses: func(context.Context, string) ([]*me.AddressEntity, error) {
			return []*me.AddressEntity{{ID: "address-1", UserID: testUserID, Street: "Rua A", IsDefault: true}}, nil
		},
	})

	recorder := serve(handler, request(http.MethodGet, "/me/addresses", ""))

	body := decode[[]map[string]any](t, recorder)
	if len(body) != 1 {
		t.Fatalf("body = %v", body)
	}
	address := body[0]
	if address["id"] != "address-1" || address["street"] != "Rua A" || address["is_default"] != true {
		t.Fatalf("address = %v", address)
	}
	if value, ok := address["label"]; !ok || value != nil {
		t.Fatalf("label = %v (present: %v), want null", value, ok)
	}
}

const addressBody = `{"label":"Casa","receiver":"Maria","zip_code":"01001000","street":"Rua A","number":"10",` +
	`"complement":null,"district":"Sé","city":"São Paulo","state":"SP","country":"BR","is_default":true}`

func TestAddAddressDecodesRequest(t *testing.T) {
	var received me.AddressInput
	handler := NewHandler(fakeService{
		addAddress: func(_ context.Context, _ string, input me.AddressInput) (*me.AddressEntity, error) {
			received = input
			return &me.AddressEntity{ID: "address-1", UserID: testUserID, Street: input.Street}, nil
		},
	})

	recorder := serve(handler, request(http.MethodPost, "/me/addresses", addressBody))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if received.Label == nil || *received.Label != "Casa" || received.Complement != nil || received.ZIPCode != "01001000" ||
		received.State != "SP" || !received.IsDefault {
		t.Fatalf("input = %+v", received)
	}
	if body := decode[map[string]any](t, recorder); body["id"] != "address-1" {
		t.Fatalf("body = %v", body)
	}
}

func TestEditAddressPassesAddressID(t *testing.T) {
	var receivedID string
	handler := NewHandler(fakeService{
		editAddress: func(_ context.Context, _ string, addressID string, input me.AddressInput) (*me.AddressEntity, error) {
			receivedID = addressID
			return &me.AddressEntity{ID: addressID, Street: input.Street}, nil
		},
	})

	recorder := serve(handler, request(http.MethodPatch, "/me/addresses/address-7", addressBody))

	if recorder.Code != http.StatusOK || receivedID != "address-7" {
		t.Fatalf("status = %d, address ID = %q", recorder.Code, receivedID)
	}
}

func TestAddressEndpointsMapNotFound(t *testing.T) {
	handler := NewHandler(fakeService{
		editAddress: func(context.Context, string, string, me.AddressInput) (*me.AddressEntity, error) {
			return nil, app.ErrAddressNotFound
		},
		deleteAddress: func(context.Context, string, string) error { return app.ErrAddressNotFound },
	})
	endpoints := map[string]endpoint{
		"edit":   {http.MethodPatch, "/me/addresses/other-users", addressBody},
		"delete": {http.MethodDelete, "/me/addresses/other-users", ""},
	}

	for name, e := range endpoints {
		t.Run(name, func(t *testing.T) {
			recorder := serve(handler, request(e.method, e.target, e.body))

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}
}

func TestDeleteAddressReturnsNoContent(t *testing.T) {
	var receivedID string
	handler := NewHandler(fakeService{
		deleteAddress: func(_ context.Context, _ string, addressID string) error {
			receivedID = addressID
			return nil
		},
	})

	recorder := serve(handler, request(http.MethodDelete, "/me/addresses/address-1", ""))

	if recorder.Code != http.StatusNoContent || receivedID != "address-1" {
		t.Fatalf("status = %d, address ID = %q", recorder.Code, receivedID)
	}
}

func TestGetPodologicalProfileReturnsNullsWhenEmpty(t *testing.T) {
	handler := NewHandler(fakeService{
		getPodologicalProfile: func(context.Context, string) (*me.PodologicalProfileEntity, error) {
			return &me.PodologicalProfileEntity{}, nil
		},
	})

	recorder := serve(handler, request(http.MethodGet, "/me/podological-profile", ""))

	want := `{"footstrike_type":null,"clinical_condition":null,"obs":null,"pain_points":[],"updated_at":null}`
	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != want {
		t.Fatalf("status = %d, body = %s, want %s", recorder.Code, recorder.Body.String(), want)
	}
}

func TestGetConsentsReturnsList(t *testing.T) {
	grantedAt := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	handler := NewHandler(fakeService{
		getConsents: func(context.Context, string) ([]*me.ConsentEntity, error) {
			return []*me.ConsentEntity{{Purpose: me.ConsentPurposePodologicalProfile, TermsVersion: "podological-profile-v1", GrantedAt: grantedAt}}, nil
		},
	})

	recorder := serve(handler, request(http.MethodGet, "/me/consents", ""))

	want := `[{"purpose":"podological_profile","terms_version":"podological-profile-v1","granted_at":"2026-10-08T12:00:00Z"}]`
	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != want {
		t.Fatalf("status = %d, body = %s, want %s", recorder.Code, recorder.Body.String(), want)
	}
}

func TestGetOnboardingReturnsStatusAndStep(t *testing.T) {
	handler := NewHandler(fakeService{
		getOnboarding: func(context.Context, string) (*me.OnboardingEntity, error) {
			return &me.OnboardingEntity{Status: me.OnboardingStatusInProgress, CurrentStep: me.OnboardingStepEntity{Tag: "address_setup"}}, nil
		},
	})

	recorder := serve(handler, request(http.MethodGet, "/me/onboarding", ""))

	want := `{"status":"in_progress","current_step":"address_setup"}`
	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != want {
		t.Fatalf("status = %d, body = %s, want %s", recorder.Code, recorder.Body.String(), want)
	}
}

func TestChangeOnboardingStepDecodesRequest(t *testing.T) {
	var received me.OnboardingInput
	handler := NewHandler(fakeService{
		changeOnboardingStep: func(_ context.Context, _ string, input me.OnboardingInput) error {
			received = input
			return nil
		},
	})

	serve(handler, request(http.MethodPut, "/me/onboarding", `{"step":"address_setup","action":"skip"}`))

	if received != (me.OnboardingInput{Step: "address_setup", Action: "skip"}) {
		t.Fatalf("input = %+v", received)
	}
}
