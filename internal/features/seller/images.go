package seller

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

const (
	MaxImages          = 6
	maxAltTextLength   = 120
	advertisementsPath = "ads"
)

type ImageInput struct {
	Content io.Reader
	AltText string
}

type ImageOrderInput struct {
	ImageIDs     []string
	CoverImageID *string
}

type AddImageUseCase interface {
	AddImage(ctx context.Context, sellerID string, id string, input ImageInput) (*AdvertisementEntity, error)
}

type RemoveImageUseCase interface {
	RemoveImage(ctx context.Context, sellerID string, id string, imageID string) (*AdvertisementEntity, error)
}

type ReorderImagesUseCase interface {
	ReorderImages(ctx context.Context, sellerID string, id string, input ImageOrderInput) (*AdvertisementEntity, error)
}

func (s *service) AddImage(ctx context.Context, sellerID string, id string, input ImageInput) (*AdvertisementEntity, error) {
	ad, err := s.ownedAdvertisement(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}

	altText := trimToNil(input.AltText)
	if altText != nil && utf8.RuneCountInString(*altText) > maxAltTextLength {
		return nil, invalid("alt_text must have at most %d characters", maxAltTextLength)
	}
	if len(ad.Images) >= MaxImages {
		return nil, app.ErrTooManyImages
	}

	upload, err := storage.ReadUpload(input.Content, app.MaxUploadSize, storage.Images)
	if err != nil {
		return nil, err
	}
	key, err := s.files.Save(ctx, storage.AreaPublic, advertisementsPath, upload.Extension, upload.Reader())
	if err != nil {
		slog.ErrorContext(ctx, "AddImage could not save file", "advertisement_id", id, "error", err)
		return nil, err
	}

	if err := s.r.AddImage(ctx, id, storage.PublicPath(key), altText, MaxImages); err != nil {
		s.deleteFile(ctx, key)
		if !isExpected(err, app.ErrTooManyImages, app.ErrAdvertisementNotFound) {
			slog.ErrorContext(ctx, "AddImage failed", "advertisement_id", id, "error", err)
		}
		return nil, err
	}
	return s.r.GetAdvertisementByID(ctx, id)
}

func (s *service) RemoveImage(ctx context.Context, sellerID string, id string, imageID string) (*AdvertisementEntity, error) {
	ad, err := s.ownedAdvertisement(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}

	index := slices.IndexFunc(ad.Images, func(i ImageEntity) bool { return i.ID == imageID })
	if index < 0 {
		return nil, app.ErrImageNotFound
	}
	if ad.Status == AdvertisementStatusPublished && len(ad.Images) == 1 {
		return nil, &app.MissingFieldsError{Fields: []string{"images"}}
	}

	if err := s.r.RemoveImage(ctx, id, imageID); err != nil {
		if !isExpected(err, app.ErrImageNotFound) {
			slog.ErrorContext(ctx, "RemoveImage failed", "advertisement_id", id, "image_id", imageID, "error", err)
		}
		return nil, err
	}
	if key, ok := storage.KeyFromPublicPath(ad.Images[index].URL); ok {
		s.deleteFile(ctx, key)
	}
	return s.r.GetAdvertisementByID(ctx, id)
}

func (s *service) ReorderImages(ctx context.Context, sellerID string, id string, input ImageOrderInput) (*AdvertisementEntity, error) {
	ad, err := s.ownedAdvertisement(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}

	current := make([]string, len(ad.Images))
	coverID := ""
	for i, image := range ad.Images {
		current[i] = image.ID
		if image.IsCover {
			coverID = image.ID
		}
	}
	ordered := slices.Clone(input.ImageIDs)
	slices.Sort(ordered)
	slices.Sort(current)
	if !slices.Equal(ordered, current) || len(current) == 0 {
		return nil, invalid("image_ids must list every image of the ad once")
	}

	if input.CoverImageID != nil {
		coverID = strings.TrimSpace(*input.CoverImageID)
		if !slices.Contains(input.ImageIDs, coverID) {
			return nil, invalid("cover_image_id must be one of the ad's images")
		}
	}
	if coverID == "" {
		coverID = input.ImageIDs[0]
	}

	if err := s.r.ReorderImages(ctx, id, input.ImageIDs, coverID); err != nil {
		slog.ErrorContext(ctx, "ReorderImages failed", "advertisement_id", id, "error", err)
		return nil, err
	}
	return s.r.GetAdvertisementByID(ctx, id)
}

func (s *service) deleteFile(ctx context.Context, key string) {
	if err := s.files.Delete(ctx, storage.AreaPublic, key); err != nil {
		slog.ErrorContext(ctx, "could not delete file", "key", key, "error", err)
	}
}
