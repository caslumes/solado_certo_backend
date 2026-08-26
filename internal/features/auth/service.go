package auth

type repository interface {}

type service struct {
	r repository
}

func NewService(r repository) *service {
	return &service{
		r: r,
	}
}