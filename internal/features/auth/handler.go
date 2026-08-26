package auth

type serviceI interface {}

type handler struct {
	svc *serviceI
}

func NewHandler(svc *serviceI) *handler {
	return &handler{
		svc: svc,
	}
}