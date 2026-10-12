package catalog

type GetAdvertisementsInput struct {
	Page    uint
	Limit   uint
	Sort    AdvertisementSort
	Filters *AdvertisementFilters
	ForMe  bool
	UserID string
}
