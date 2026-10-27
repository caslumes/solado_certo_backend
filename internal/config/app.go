package config

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	authRepository "github.com/caslumes/solado_certo_backend/internal/features/auth/repository/postgres"
	httpauth "github.com/caslumes/solado_certo_backend/internal/features/auth/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	catalogRepository "github.com/caslumes/solado_certo_backend/internal/features/catalog/repository/postgres"
	httpcatalog "github.com/caslumes/solado_certo_backend/internal/features/catalog/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/customorders"
	customordersRepository "github.com/caslumes/solado_certo_backend/internal/features/customorders/repository/postgres"
	httpcustomorders "github.com/caslumes/solado_certo_backend/internal/features/customorders/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/documents"
	documentsRepository "github.com/caslumes/solado_certo_backend/internal/features/documents/repository/postgres"
	httpdocuments "github.com/caslumes/solado_certo_backend/internal/features/documents/transport/http"
	httpfiles "github.com/caslumes/solado_certo_backend/internal/features/files/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	meRepository "github.com/caslumes/solado_certo_backend/internal/features/me/repository/postgres"
	httpme "github.com/caslumes/solado_certo_backend/internal/features/me/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	notificationsRepository "github.com/caslumes/solado_certo_backend/internal/features/notifications/repository/postgres"
	httpnotifications "github.com/caslumes/solado_certo_backend/internal/features/notifications/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	ordersRepository "github.com/caslumes/solado_certo_backend/internal/features/orders/repository/postgres"
	httporders "github.com/caslumes/solado_certo_backend/internal/features/orders/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/payments"
	paymentsRepository "github.com/caslumes/solado_certo_backend/internal/features/payments/repository/postgres"
	httppayments "github.com/caslumes/solado_certo_backend/internal/features/payments/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/reviews"
	reviewsRepository "github.com/caslumes/solado_certo_backend/internal/features/reviews/repository/postgres"
	httpreviews "github.com/caslumes/solado_certo_backend/internal/features/reviews/transport/http"
	"github.com/caslumes/solado_certo_backend/internal/features/seller"
	sellerRepository "github.com/caslumes/solado_certo_backend/internal/features/seller/repository/postgres"
	httpseller "github.com/caslumes/solado_certo_backend/internal/features/seller/transport/http"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	rolemiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/role"
	"github.com/caslumes/solado_certo_backend/pkg/gateway"
	"github.com/caslumes/solado_certo_backend/pkg/gateway/asaas"
	"github.com/caslumes/solado_certo_backend/pkg/gateway/fake"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

const fakeConfirmAfter = 30 * time.Second

type App struct {
	Envs           *Envs
	DB             *pgxpool.Pool
	AuthMiddleware *authmiddleware.AuthMiddleware
	RoleMiddleware *rolemiddleware.RoleMiddleware
	AuthHandler    *httpauth.Handler
	MeHandler      *httpme.Handler
	CatalogHandler *httpcatalog.Handler
	FilesHandler   *httpfiles.Handler
	SellerHandler  *httpseller.Handler
	OrdersHandler  *httporders.Handler

	PaymentsHandler      *httppayments.Handler
	PaymentPoller        payments.CheckPaymentsUseCase
	DeliveryJob          orders.DeliverOverdueOrdersUseCase
	NotificationsHandler *httpnotifications.Handler
	ReviewsHandler       *httpreviews.Handler
	DocumentsHandler     *httpdocuments.Handler
	CustomOrdersHandler  *httpcustomorders.Handler
}

func InitApp(ctx context.Context) (*App, error) {
	envs, err := LoadEnvs()
	if err != nil {
		return nil, err
	}

	dbPool, err := InitDatabase(ctx, envs.DatabaseURL)
	if err != nil {
		return nil, err
	}

	fileStorage, err := storage.NewLocalStorage(envs.UploadsDir)
	if err != nil {
		return nil, err
	}

	authService := auth.NewService(authRepository.NewPostgresRepository(dbPool), envs.SecretKey)
	meService := me.NewService(meRepository.NewPostgresRepository(dbPool), fileStorage)
	sellerService := seller.NewService(sellerRepository.NewPostgresRepository(dbPool), fileStorage)
	ordersService := orders.NewService(ordersRepository.NewPostgresRepository(dbPool))
	catalogService := catalog.NewService(catalogRepository.NewPostgresRepository(dbPool), meService)
	paymentsService := payments.NewService(paymentsRepository.NewPostgresRepository(dbPool), newPaymentGateway(envs))
	ordersService.SetRefunder(paymentsService)
	notificationsService := notifications.NewService(notificationsRepository.NewPostgresRepository(dbPool))
	reviewsService := reviews.NewService(reviewsRepository.NewPostgresRepository(dbPool))
	documentsService := documents.NewService(documentsRepository.NewPostgresRepository(dbPool), fileStorage, meService)
	customOrdersService := customorders.NewService(customordersRepository.NewPostgresRepository(dbPool), meService, ordersService, fileStorage)

	return &App{
		Envs:           envs,
		DB:             dbPool,
		AuthMiddleware: authmiddleware.NewAuthMiddleware(authService),
		RoleMiddleware: rolemiddleware.NewRoleMiddleware(meService),
		AuthHandler:    httpauth.NewHandler(authService),
		MeHandler:      httpme.NewHandler(meService),
		CatalogHandler: httpcatalog.NewHandler(catalogService),
		FilesHandler:   httpfiles.NewHandler(fileStorage),
		SellerHandler:  httpseller.NewHandler(sellerService),
		OrdersHandler:  httporders.NewHandler(ordersService),

		PaymentsHandler:      httppayments.NewHandler(paymentsService, envs.AsaasWebhookToken),
		PaymentPoller:        paymentsService,
		DeliveryJob:          ordersService,
		NotificationsHandler: httpnotifications.NewHandler(notificationsService),
		ReviewsHandler:       httpreviews.NewHandler(reviewsService),
		DocumentsHandler:     httpdocuments.NewHandler(documentsService),
		CustomOrdersHandler:  httpcustomorders.NewHandler(customOrdersService),
	}, nil
}

func newPaymentGateway(envs *Envs) gateway.PaymentGateway {
	if envs.PaymentGateway == PaymentGatewayFake {
		slog.Warn("using the fake payment gateway: no real charges are made")
		return fake.New(fakeConfirmAfter)
	}
	if envs.AsaasWebhookToken == "" {
		slog.Warn("ASAAS_WEBHOOK_TOKEN is not set: webhooks are refused and payment statuses come from polling only")
	}
	slog.Info("using the Asaas payment gateway", "base_url", envs.AsaasBaseURL)
	return asaas.New(envs.AsaasBaseURL, envs.AsaasAPIKey)
}
