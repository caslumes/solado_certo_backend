package app

import "errors"

var ErrInvalidInput = errors.New("invalid input")
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrUserNotFound = errors.New("user not found")
var ErrTokenNotFound = errors.New("token not found")
var ErrTokenRevoked = errors.New("token revoked")
var ErrAddressNotFound = errors.New("address not found")
var ErrPodologicalProfileNotFound = errors.New("podological profile not found")
var ErrOnboardingNotFound = errors.New("onboarding not found")
var ErrOnboardingStepNotFound = errors.New("onboarding step not found")
var ErrOnboardingAlreadyCompleted = errors.New("onboarding already completed")
