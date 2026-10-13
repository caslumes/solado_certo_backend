package seller

// AdvertisementInput is used to create and to edit an ad. On edit, a nil field keeps its value;
// an empty string clears an optional text field and an empty list clears a list.
type AdvertisementInput struct {
	CategoryID           *string
	Title                *string
	Description          *string
	Price                *string
	Stock                *int
	PrescriptionRequired *bool
	CustomMadeAllowed    *bool
	Side                 *string
	Size                 *string
	Material             *string
	Rigidity             *string
	UsageIndication      *string
	SafetyAlerts         *string
	Contraindications    *string
	ShippingNotes        *string
	PurposeCodes         *[]string
	ConditionIDs         *[]string
	ShippingOptions      *[]ShippingOptionInput
	// Publish asks to create the ad already published (create only).
	Publish bool
}

// ShippingOptionInput without an ID is a new option; with an ID it edits one of the ad's options.
type ShippingOptionInput struct {
	ID           *string
	Name         string
	Carrier      *string
	Price        string
	DeliveryDays int
}
