package seller

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
	Publish              bool
}

type ShippingOptionInput struct {
	ID           *string
	Name         string
	Carrier      *string
	Price        string
	DeliveryDays int
}
