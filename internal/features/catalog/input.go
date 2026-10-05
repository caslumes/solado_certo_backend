package catalog

type GetAdvertisementsInput struct {
	Page    uint
	Limit   uint
	Filters *AdvertisementFilters
}
