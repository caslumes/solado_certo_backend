package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/config"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/internal/features/payments"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

const shutdownTimeout = 10 * time.Second

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := config.InitApp(ctx)
	if err != nil {
		return err
	}
	defer app.DB.Close()

	server := &http.Server{
		Addr:              ":" + app.Envs.ServerPort,
		Handler:           newRouter(app),
		ReadHeaderTimeout: 5 * time.Second,
	}

	var jobs sync.WaitGroup
	jobs.Go(func() { payments.RunPoller(ctx, app.PaymentPoller, payments.PollInterval) })
	jobs.Go(func() { orders.RunDeliveryJob(ctx, app.DeliveryJob, orders.DeliveryJobInterval) })

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", server.Addr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		stop()
		jobs.Wait()
		return err
	case <-ctx.Done():
	}

	stop()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	jobs.Wait()
	app.PaymentsHandler.Wait()
	app.PendingMail.WaitForMail()
	return nil
}

func newRouter(app *config.App) *gin.Engine {
	router := gin.Default()
	router.SetTrustedProxies(nil)

	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{"http://localhost:*", "http://127.0.0.1:*"},
		AllowOriginFunc: func(origin string) bool {
			return strings.HasPrefix(origin, "http://localhost:") ||
				strings.HasPrefix(origin, "http://127.0.0.1:")
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "Idempotency-Key"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	initRoutes(router, app)

	return router
}
