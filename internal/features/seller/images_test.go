package seller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

var pngBytes = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0x0D}

func (f *fakeRepository) AddImage(_ context.Context, advertisementID string, url string, altText *string, maxImages int) error {
	ad := f.ads[advertisementID]
	if len(ad.Images) >= maxImages {
		return app.ErrTooManyImages
	}
	f.nextID++
	image := ImageEntity{ID: fmt.Sprintf("image-%d", f.nextID), URL: url, Position: len(ad.Images) + 1, IsCover: len(ad.Images) == 0}
	if altText != nil {
		image.AltText = *altText
	}
	ad.Images = append(ad.Images, image)
	return nil
}

func (f *fakeRepository) RemoveImage(_ context.Context, advertisementID string, imageID string) error {
	ad := f.ads[advertisementID]
	ad.Images = slices.DeleteFunc(ad.Images, func(i ImageEntity) bool { return i.ID == imageID })
	hasCover := false
	for i := range ad.Images {
		ad.Images[i].Position = i + 1
		hasCover = hasCover || ad.Images[i].IsCover
	}
	if !hasCover && len(ad.Images) > 0 {
		ad.Images[0].IsCover = true
	}
	return nil
}

func (f *fakeRepository) ReorderImages(_ context.Context, advertisementID string, imageIDs []string, coverImageID string) error {
	ad := f.ads[advertisementID]
	images := make([]ImageEntity, len(imageIDs))
	for i, id := range imageIDs {
		index := slices.IndexFunc(ad.Images, func(image ImageEntity) bool { return image.ID == id })
		images[i] = ad.Images[index]
		images[i].Position = i + 1
		images[i].IsCover = id == coverImageID
	}
	ad.Images = images
	return nil
}

func newImageService(t *testing.T, status AdvertisementStatus, images int) (*service, *fakeRepository, *storage.MemoryStorage, *AdvertisementEntity) {
	t.Helper()
	repository := newFakeRepository()
	files := storage.NewMemoryStorage()
	ad := adWithStatus(repository, status)
	repository.ads[ad.ID].Images = nil
	s := NewService(repository, files)
	for range images {
		if _, err := s.AddImage(context.Background(), testSellerID, ad.ID, ImageInput{Content: bytes.NewReader(pngBytes)}); err != nil {
			t.Fatalf("AddImage: %v", err)
		}
	}
	return s, repository, files, ad
}

func imageIDs(images []ImageEntity) []string {
	ids := make([]string, len(images))
	for i, image := range images {
		ids[i] = image.ID
	}
	return ids
}

func TestAddImageStoresFileAndMakesFirstTheCover(t *testing.T) {
	s, _, files, ad := newImageService(t, AdvertisementStatusDraft, 0)

	updated, err := s.AddImage(context.Background(), testSellerID, ad.ID, ImageInput{Content: bytes.NewReader(pngBytes), AltText: " Frente "})
	if err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	if len(updated.Images) != 1 || !updated.Images[0].IsCover || updated.Images[0].AltText != "Frente" {
		t.Fatalf("images = %+v", updated.Images)
	}
	key, ok := storage.KeyFromPublicPath(updated.Images[0].URL)
	if !ok || !strings.HasPrefix(key, "ads/") || !strings.HasSuffix(key, ".png") || !files.Has(storage.AreaPublic, key) {
		t.Fatalf("url = %q was not saved as a public file", updated.Images[0].URL)
	}

	second, err := s.AddImage(context.Background(), testSellerID, ad.ID, ImageInput{Content: bytes.NewReader(pngBytes)})
	if err != nil {
		t.Fatalf("AddImage: %v", err)
	}
	if second.Images[1].IsCover || second.Images[1].Position != 2 {
		t.Fatalf("second image = %+v", second.Images[1])
	}
}

func TestAddImageRejectsInvalidFiles(t *testing.T) {
	tests := map[string]struct {
		content []byte
		want    error
	}{
		"text named like an image": {[]byte("not an image"), storage.ErrUnsupportedType},
		"pdf":                      {[]byte("%PDF-1.7\n"), storage.ErrUnsupportedType},
		"over 5 MB":                {append(slices.Clone(pngBytes), make([]byte, app.MaxUploadSize)...), storage.ErrTooLarge},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			s, repository, files, ad := newImageService(t, AdvertisementStatusDraft, 0)

			_, err := s.AddImage(context.Background(), testSellerID, ad.ID, ImageInput{Content: bytes.NewReader(test.content)})

			if !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}
			if files.Count() != 0 || len(repository.ads[ad.ID].Images) != 0 {
				t.Fatal("the file was stored")
			}
		})
	}
}

func TestAddImageRejectsLongAltText(t *testing.T) {
	s, _, _, ad := newImageService(t, AdvertisementStatusDraft, 0)

	_, err := s.AddImage(context.Background(), testSellerID, ad.ID, ImageInput{Content: bytes.NewReader(pngBytes), AltText: strings.Repeat("a", 121)})

	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("err = %v, want %v", err, app.ErrInvalidInput)
	}
}

func TestAddImageAllowsAtMostSixImages(t *testing.T) {
	s, _, files, ad := newImageService(t, AdvertisementStatusDraft, MaxImages)

	_, err := s.AddImage(context.Background(), testSellerID, ad.ID, ImageInput{Content: bytes.NewReader(pngBytes)})

	if !errors.Is(err, app.ErrTooManyImages) {
		t.Fatalf("err = %v, want %v", err, app.ErrTooManyImages)
	}
	if files.Count() != MaxImages {
		t.Fatalf("files = %d, want %d", files.Count(), MaxImages)
	}
}

type fullRepository struct {
	*fakeRepository
}

func (fullRepository) AddImage(context.Context, string, string, *string, int) error {
	return app.ErrTooManyImages
}

func TestAddImageDeletesFileWhenSavingTheRowFails(t *testing.T) {
	repository := newFakeRepository()
	ad := adWithStatus(repository, AdvertisementStatusDraft)
	files := storage.NewMemoryStorage()

	_, err := NewService(fullRepository{repository}, files).AddImage(context.Background(), testSellerID, ad.ID, ImageInput{Content: bytes.NewReader(pngBytes)})

	if !errors.Is(err, app.ErrTooManyImages) {
		t.Fatalf("err = %v, want %v", err, app.ErrTooManyImages)
	}
	if files.Count() != 0 {
		t.Fatal("the file was left behind")
	}
}

func TestImageEndpointsCheckOwnership(t *testing.T) {
	s, repository, _, ad := newImageService(t, AdvertisementStatusDraft, 1)
	repository.ads[ad.ID].SellerID = otherSellerID
	ctx := context.Background()
	imageID := repository.ads[ad.ID].Images[0].ID

	calls := map[string]func() error{
		"add": func() error {
			_, err := s.AddImage(ctx, testSellerID, ad.ID, ImageInput{Content: bytes.NewReader(pngBytes)})
			return err
		},
		"remove": func() error { _, err := s.RemoveImage(ctx, testSellerID, ad.ID, imageID); return err },
		"reorder": func() error {
			_, err := s.ReorderImages(ctx, testSellerID, ad.ID, ImageOrderInput{ImageIDs: []string{imageID}})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, app.ErrForbidden) {
				t.Fatalf("err = %v, want %v", err, app.ErrForbidden)
			}
		})
	}
}

func TestRemoveImageDeletesFileAndMovesCover(t *testing.T) {
	s, repository, files, ad := newImageService(t, AdvertisementStatusDraft, 2)
	cover := repository.ads[ad.ID].Images[0]
	key, _ := storage.KeyFromPublicPath(cover.URL)

	updated, err := s.RemoveImage(context.Background(), testSellerID, ad.ID, cover.ID)
	if err != nil {
		t.Fatalf("RemoveImage: %v", err)
	}

	if len(updated.Images) != 1 || !updated.Images[0].IsCover || updated.Images[0].Position != 1 {
		t.Fatalf("images = %+v", updated.Images)
	}
	if files.Has(storage.AreaPublic, key) {
		t.Fatal("the file was not deleted")
	}
}

func TestRemoveImageKeepsFilesThatAreNotOurs(t *testing.T) {
	s, repository, files, ad := newImageService(t, AdvertisementStatusDraft, 1)
	repository.ads[ad.ID].Images = append(repository.ads[ad.ID].Images, ImageEntity{ID: "seed", URL: "https://placehold.co/600", Position: 2})

	if _, err := s.RemoveImage(context.Background(), testSellerID, ad.ID, "seed"); err != nil {
		t.Fatalf("RemoveImage: %v", err)
	}
	if files.Count() != 1 {
		t.Fatalf("files = %d, want the uploaded one kept", files.Count())
	}
}

func TestRemoveImageReturnsNotFound(t *testing.T) {
	s, _, _, ad := newImageService(t, AdvertisementStatusDraft, 1)

	if _, err := s.RemoveImage(context.Background(), testSellerID, ad.ID, "image-99"); !errors.Is(err, app.ErrImageNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrImageNotFound)
	}
}

func TestRemoveImageKeepsLastPhotoOfPublishedAd(t *testing.T) {
	s, repository, files, ad := newImageService(t, AdvertisementStatusPublished, 1)

	_, err := s.RemoveImage(context.Background(), testSellerID, ad.ID, repository.ads[ad.ID].Images[0].ID)

	var missing *app.MissingFieldsError
	if !errors.As(err, &missing) || !slices.Equal(missing.Fields, []string{"images"}) {
		t.Fatalf("err = %v, want missing images", err)
	}
	if files.Count() != 1 || len(repository.ads[ad.ID].Images) != 1 {
		t.Fatal("the photo was removed")
	}
}

func TestReorderImagesSetsOrderAndCover(t *testing.T) {
	s, repository, _, ad := newImageService(t, AdvertisementStatusDraft, 3)
	ids := imageIDs(repository.ads[ad.ID].Images)
	order := []string{ids[2], ids[0], ids[1]}

	updated, err := s.ReorderImages(context.Background(), testSellerID, ad.ID, ImageOrderInput{ImageIDs: order, CoverImageID: &ids[1]})
	if err != nil {
		t.Fatalf("ReorderImages: %v", err)
	}

	if !slices.Equal(imageIDs(updated.Images), order) || !updated.Images[2].IsCover || updated.Images[0].IsCover {
		t.Fatalf("images = %+v", updated.Images)
	}
}

func TestReorderImagesKeepsCoverWhenNotGiven(t *testing.T) {
	s, repository, _, ad := newImageService(t, AdvertisementStatusDraft, 2)
	ids := imageIDs(repository.ads[ad.ID].Images)

	updated, err := s.ReorderImages(context.Background(), testSellerID, ad.ID, ImageOrderInput{ImageIDs: []string{ids[1], ids[0]}})
	if err != nil {
		t.Fatalf("ReorderImages: %v", err)
	}

	if updated.Images[1].ID != ids[0] || !updated.Images[1].IsCover {
		t.Fatalf("images = %+v, want the old cover kept", updated.Images)
	}
}

func TestReorderImagesRejectsInvalidLists(t *testing.T) {
	s, repository, _, ad := newImageService(t, AdvertisementStatusDraft, 2)
	ids := imageIDs(repository.ads[ad.ID].Images)
	other := "image-99"

	tests := map[string]ImageOrderInput{
		"missing an image": {ImageIDs: ids[:1]},
		"repeated image":   {ImageIDs: []string{ids[0], ids[0]}},
		"unknown image":    {ImageIDs: []string{ids[0], other}},
		"extra image":      {ImageIDs: append(slices.Clone(ids), other)},
		"cover not listed": {ImageIDs: ids, CoverImageID: &other},
		"empty":            {},
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := s.ReorderImages(context.Background(), testSellerID, ad.ID, input); !errors.Is(err, app.ErrInvalidInput) {
				t.Fatalf("err = %v, want %v", err, app.ErrInvalidInput)
			}
		})
	}
}
