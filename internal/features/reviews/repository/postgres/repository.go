package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/reviews"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const uniqueViolation = "23505"

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) GetOrderItem(ctx context.Context, orderID string, itemID string) (*reviews.ReviewedItem, error) {
	var item reviews.ReviewedItem
	var id *string
	err := r.pool.QueryRow(
		ctx,
		"SELECT oi.id, o.id, o.buyer_id, o.status FROM orders o "+
			"LEFT JOIN order_items oi ON oi.order_id = o.id AND oi.id = $2 WHERE o.id = $1",
		orderID, itemID,
	).Scan(&id, &item.OrderID, &item.BuyerID, &item.OrderStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrOrderNotFound
		}
		return nil, err
	}
	if id != nil {
		item.ID = *id
	}
	return &item, nil
}

func (r *postgresRepository) CreateReview(ctx context.Context, review reviews.NewReview) (*reviews.ReviewEntity, error) {
	now := time.Now().UTC()
	created := reviews.ReviewEntity{
		OrderItemID:        review.OrderItemID,
		ProductRating:      review.ProductRating,
		SellerRating:       review.SellerRating,
		ComfortRating:      review.ComfortRating,
		Comment:            review.Comment,
		AdaptationFeedback: review.AdaptationFeedback,
		CreatedAt:          now,
	}
	err := r.pool.QueryRow(
		ctx,
		"INSERT INTO reviews (order_item_id, reviewer_id, product_rating, seller_rating, comfort_rating, comment, adaptation_feedback, created_at, updated_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8) RETURNING id",
		review.OrderItemID, review.ReviewerID, review.ProductRating, review.SellerRating, review.ComfortRating,
		review.Comment, review.AdaptationFeedback, now,
	).Scan(&created.ID)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == uniqueViolation {
		return nil, app.ErrAlreadyReviewed
	}
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func (r *postgresRepository) CountReviews(ctx context.Context, advertisementID string) (uint, error) {
	var total uint
	err := r.pool.QueryRow(
		ctx,
		"SELECT count(*) FROM reviews rv JOIN order_items oi ON oi.id = rv.order_item_id WHERE oi.advertisement_id = $1",
		advertisementID,
	).Scan(&total)
	return total, err
}

func (r *postgresRepository) GetReviews(ctx context.Context, advertisementID string, limit uint, offset uint) ([]*reviews.PublicReviewEntity, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT rv.id, split_part(u.name, ' ', 1), rv.product_rating, rv.seller_rating, rv.comfort_rating, rv.comment, rv.created_at "+
			"FROM reviews rv JOIN order_items oi ON oi.id = rv.order_item_id JOIN users u ON u.id = rv.reviewer_id "+
			"WHERE oi.advertisement_id = $1 ORDER BY rv.created_at DESC, rv.id DESC LIMIT $2 OFFSET $3",
		advertisementID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*reviews.PublicReviewEntity, error) {
		var rv reviews.PublicReviewEntity
		err := row.Scan(&rv.ID, &rv.ReviewerName, &rv.ProductRating, &rv.SellerRating, &rv.ComfortRating, &rv.Comment, &rv.CreatedAt)
		return &rv, err
	})
}
