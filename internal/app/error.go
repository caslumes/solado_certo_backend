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
var ErrCartItemNotFound = errors.New("cart item not found")
var ErrOwnAdvertisement = errors.New("cannot buy your own advertisement")
var ErrOutOfStock = errors.New("out of stock")
var ErrCartEmpty = errors.New("cart is empty")
var ErrCartChanged = errors.New("cart changed")
var ErrPricesChanged = errors.New("prices changed")
var ErrShippingRequired = errors.New("shipping option required")
var ErrOrderNotFound = errors.New("order not found")
var ErrOrderExpired = errors.New("order payment window expired")
var ErrPaymentNotFound = errors.New("payment not found")
var ErrPaymentInProgress = errors.New("another payment of this order is in progress")
var ErrTaxIDRequired = errors.New("tax id required")
var ErrTaxIDInUse = errors.New("tax id already registered")
var ErrIdempotencyKeyReused = errors.New("idempotency key already used")
var ErrGatewayUnavailable = errors.New("payment gateway unavailable")
var ErrNotificationNotFound = errors.New("notification not found")
var ErrInvalidTrackingCode = errors.New("invalid tracking code")
var ErrTrackingCodeInUse = errors.New("tracking code already registered")
var ErrOrderItemNotFound = errors.New("order item not found")
var ErrReviewNotAllowed = errors.New("only a delivered order can be reviewed")
var ErrAlreadyReviewed = errors.New("item already reviewed")

type MissingFieldsError struct {
	Fields []string
}

func (e *MissingFieldsError) Error() string {
	return ErrMissingRequiredFields.Error() + ": " + strings.Join(e.Fields, ", ")
}

func (e *MissingFieldsError) Unwrap() error {
	return ErrMissingRequiredFields
}

type StockShortage struct {
	AdvertisementID string `json:"advertisement_id"`
	Requested       int    `json:"requested"`
	Available       int    `json:"available"`
}

type OutOfStockError struct {
	Items []StockShortage
}

func (e *OutOfStockError) Error() string {
	return ErrOutOfStock.Error()
}

func (e *OutOfStockError) Unwrap() error {
	return ErrOutOfStock
}

type ShippingRequiredError struct {
	AdvertisementIDs []string
}

func (e *ShippingRequiredError) Error() string {
	return ErrShippingRequired.Error() + ": " + strings.Join(e.AdvertisementIDs, ", ")
}

func (e *ShippingRequiredError) Unwrap() error {
	return ErrShippingRequired
}

var ErrPrescriptionRequired = errors.New("prescription required")
var ErrPrescriptionNotNeeded = errors.New("this item does not need a prescription")
var ErrDocumentNotFound = errors.New("document not found")
var ErrUnreadableDocument = errors.New("unreadable document")

type PrescriptionRequiredError struct {
	ItemIDs []string
}

func (e *PrescriptionRequiredError) Error() string {
	return ErrPrescriptionRequired.Error() + ": " + strings.Join(e.ItemIDs, ", ")
}

func (e *PrescriptionRequiredError) Unwrap() error {
	return ErrPrescriptionRequired
}

var ErrCustomOrderNotFound = errors.New("custom order request not found")
var ErrCustomMadeNotAllowed = errors.New("this advertisement does not take custom orders")
var ErrFootPhotoNotFound = errors.New("foot photo not found")
var ErrFittingReportRequired = errors.New("fitting report required")

type FittingReportRequiredError struct {
	ItemIDs []string
}

func (e *FittingReportRequiredError) Error() string {
	return ErrFittingReportRequired.Error() + ": " + strings.Join(e.ItemIDs, ", ")
}

func (e *FittingReportRequiredError) Unwrap() error {
	return ErrFittingReportRequired
}

var ErrInvalidResetCode = errors.New("invalid or expired code")
var ErrResetAttemptsExceeded = errors.New("too many attempts, request a new code")

var ErrTicketNotFound = errors.New("support ticket not found")
var ErrAttachmentNotFound = errors.New("attachment not found")
