package catalog

type AdvertisementStatus string

const (
	AdvertisementStatusDraft     AdvertisementStatus = "draft"
	AdvertisementStatusPublished AdvertisementStatus = "published"
	AdvertisementStatusPaused    AdvertisementStatus = "paused"
	AdvertisementStatusArchived  AdvertisementStatus = "archived"
	AdvertisementStatusRemoved   AdvertisementStatus = "removed"
)

type HomeSectionCode string

const (
	HomeSectionForMe    HomeSectionCode = "FOR_ME"
	HomeSectionTrending HomeSectionCode = "TRENDING"
)

type AdvertisementSort string

const (
	AdvertisementSortRelevance AdvertisementSort = "relevance"
	AdvertisementSortPriceAsc  AdvertisementSort = "price_asc"
	AdvertisementSortPriceDesc AdvertisementSort = "price_desc"
	AdvertisementSortNewest    AdvertisementSort = "newest"
)

func (s AdvertisementSort) IsValid() bool {
	switch s {
	case AdvertisementSortRelevance, AdvertisementSortPriceAsc, AdvertisementSortPriceDesc, AdvertisementSortNewest:
		return true
	}
	return false
}
