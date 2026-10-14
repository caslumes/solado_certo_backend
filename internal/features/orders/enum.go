package orders

const AdvertisementStatusPublished = "published"

type ItemProblem string

const (
	ItemProblemUnavailable       ItemProblem = "unavailable"
	ItemProblemOutOfStock        ItemProblem = "out_of_stock"
	ItemProblemInsufficientStock ItemProblem = "insufficient_stock"
)
