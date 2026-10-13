package seller

type AdvertisementStatus string

const (
	AdvertisementStatusDraft     AdvertisementStatus = "draft"
	AdvertisementStatusPublished AdvertisementStatus = "published"
	AdvertisementStatusPaused    AdvertisementStatus = "paused"
	AdvertisementStatusRemoved   AdvertisementStatus = "removed"
)

func (s AdvertisementStatus) IsListable() bool {
	switch s {
	case AdvertisementStatusDraft, AdvertisementStatusPublished, AdvertisementStatusPaused:
		return true
	}
	return false
}

type StatusAction string

const (
	StatusActionPublish    StatusAction = "publish"
	StatusActionPause      StatusAction = "pause"
	StatusActionReactivate StatusAction = "reactivate"
)

type transition struct {
	From AdvertisementStatus
	To   AdvertisementStatus
}

// Removing is not an action here: any status except removed can be removed (RemoveAdvertisement).
var transitions = map[StatusAction]transition{
	StatusActionPublish:    {From: AdvertisementStatusDraft, To: AdvertisementStatusPublished},
	StatusActionPause:      {From: AdvertisementStatusPublished, To: AdvertisementStatusPaused},
	StatusActionReactivate: {From: AdvertisementStatusPaused, To: AdvertisementStatusPublished},
}

const (
	AttributeSide            = "side"
	AttributeSize            = "size"
	AttributeMaterial        = "material"
	AttributeRigidity        = "rigidity"
	AttributeUsageIndication = "usage_indication"
)

var validSides = map[string]bool{"left": true, "right": true, "both": true}

var validRigidities = map[string]bool{"flexible": true, "semi_rigid": true, "rigid": true}
