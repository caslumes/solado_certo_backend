package app

import (
	"errors"
	"strings"
)

var ErrInvalidInput = errors.New("invalid input")
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrUserNotFound = errors.New("user not found")
var ErrUserAlreadyExists = errors.New("email or phone already registered")
var ErrTokenNotFound = errors.New("token not found")
var ErrTokenRevoked = errors.New("token revoked")
var ErrTokenExpired = errors.New("token expired")
var ErrInvalidToken = errors.New("invalid token")
var ErrAddressNotFound = errors.New("address not found")
var ErrPodologicalProfileNotFound = errors.New("podological profile not found")
var ErrConsentRequired = errors.New("consent required")
var ErrConsentNotFound = errors.New("consent not found")
var ErrOnboardingNotFound = errors.New("onboarding not found")
var ErrOnboardingStepNotFound = errors.New("onboarding step not found")
var ErrOnboardingAlreadyCompleted = errors.New("onboarding already completed")
var ErrAdvertisementNotFound = errors.New("advertisement not found")
var ErrCategoryNotFound = errors.New("category not found")
var ErrForbidden = errors.New("forbidden")
var ErrInvalidStatusTransition = errors.New("invalid status transition")
var ErrMissingRequiredFields = errors.New("missing required fields")
var ErrImageNotFound = errors.New("image not found")
var ErrTooManyImages = errors.New("too many images")

// MissingFieldsError lists the JSON names of the fields that must be filled.
type MissingFieldsError struct {
	Fields []string
}

func (e *MissingFieldsError) Error() string {
	return ErrMissingRequiredFields.Error() + ": " + strings.Join(e.Fields, ", ")
}

func (e *MissingFieldsError) Unwrap() error {
	return ErrMissingRequiredFields
}
