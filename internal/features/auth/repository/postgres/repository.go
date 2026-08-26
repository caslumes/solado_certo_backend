package repository

type postgresRepository struct{}

func NewPostgresRepository() *postgresRepository {
	return &postgresRepository{}
}
