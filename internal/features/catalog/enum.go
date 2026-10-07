package catalog

type AdvertisementStatus string

const (
	AdvertisementStatusDraft     AdvertisementStatus = "draft"
	AdvertisementStatusPublished AdvertisementStatus = "published"
	AdvertisementStatusPaused    AdvertisementStatus = "paused"
	AdvertisementStatusArchived  AdvertisementStatus = "archived"
	AdvertisementStatusRemoved   AdvertisementStatus = "removed"
)
