package catalog

type AdvertisementStatus string

const (
	AdvertisementStatusDraft     AdvertisementStatus = "rascunho"
	AdvertisementStatusPublished AdvertisementStatus = "publicado"
	AdvertisementStatusPaused    AdvertisementStatus = "pausado"
	AdvertisementStatusArchived  AdvertisementStatus = "arquivado"
	AdvertisementStatusRemoved   AdvertisementStatus = "removido"
)
