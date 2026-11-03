package main

import (
	"context"
	"net/http"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/config"
	"github.com/caslumes/solado_certo_backend/internal/middleware/ratelimit"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func initRoutes(r *gin.Engine, a *config.App) {
	r.GET("/health", health(a.DB))

	registerLimiter := ratelimit.New(5, time.Minute)
	loginLimiter := ratelimit.New(5, time.Minute)
	forgotPasswordLimiter := ratelimit.New(5, time.Minute)
	resetPasswordLimiter := ratelimit.New(5, time.Minute)

	authGroup := r.Group("/auth")
	{
		authGroup.POST("/register", registerLimiter.Middleware(), a.AuthHandler.Register)
		authGroup.POST("/login", loginLimiter.Middleware(), a.AuthHandler.Login)
		authGroup.POST("/logout", a.AuthHandler.Logout)
		authGroup.POST("/refresh-token", a.AuthHandler.RefreshToken)
		authGroup.POST("/password/forgot", forgotPasswordLimiter.Middleware(), a.AuthHandler.ForgotPassword)
		authGroup.POST("/password/reset", resetPasswordLimiter.Middleware(), a.AuthHandler.ResetPassword)
	}

	meGroup := r.Group("/me", a.AuthMiddleware.Middleware())
	{
		meGroup.GET("/profile", a.MeHandler.GetProfile)
		meGroup.PATCH("/profile", a.MeHandler.EditProfile)
		meGroup.PUT("/avatar", a.MeHandler.ChangeAvatar)
		meGroup.DELETE("/avatar", a.MeHandler.RemoveAvatar)

		meGroup.GET("/addresses", a.MeHandler.GetAddresses)
		meGroup.POST("/addresses", a.MeHandler.AddAddress)
		meGroup.PATCH("/addresses/:addressID", a.MeHandler.EditAddress)
		meGroup.DELETE("/addresses/:addressID", a.MeHandler.DeleteAddress)

		meGroup.GET("/podological-profile", a.MeHandler.GetPodologicalProfile)
		meGroup.PUT("/podological-profile", a.MeHandler.EditPodologicalProfile)
		meGroup.GET("/pain-points", a.MeHandler.GetPainPoints)

		meGroup.GET("/consents", a.MeHandler.GetConsents)
		meGroup.POST("/consents", a.MeHandler.GrantConsent)
		meGroup.DELETE("/consents/:purpose", a.MeHandler.RevokeConsent)

		meGroup.GET("/roles", a.MeHandler.GetRoles)
		meGroup.POST("/roles/seller", a.MeHandler.BecomeSeller)

		meGroup.GET("/onboarding", a.MeHandler.GetOnboarding)
		meGroup.PUT("/onboarding", a.MeHandler.ChangeOnboardingStep)
	}

	catalogGroup := r.Group("/catalog")
	{
		catalogGroup.GET("", a.AuthMiddleware.OptionalMiddleware(), a.CatalogHandler.GetAdvertisements)
		catalogGroup.GET("/items/:advertisementID", a.CatalogHandler.GetAdvertisement)
		catalogGroup.GET("/items/:advertisementID/reviews", a.ReviewsHandler.ListReviews)
		catalogGroup.GET("/categories", a.CatalogHandler.GetCategories)
		catalogGroup.GET("/purposes", a.CatalogHandler.GetPurposes)
		catalogGroup.GET("/conditions", a.CatalogHandler.GetConditions)
	}

	sellerGroup := r.Group("/seller", a.AuthMiddleware.Middleware(), a.RoleMiddleware.RequireRole(enum.RoleSeller))
	{
		sellerGroup.GET("/ads", a.SellerHandler.ListAdvertisements)
		sellerGroup.POST("/ads", a.SellerHandler.CreateAdvertisement)
		sellerGroup.GET("/ads/:advertisementID", a.SellerHandler.GetAdvertisement)
		sellerGroup.PATCH("/ads/:advertisementID", a.SellerHandler.EditAdvertisement)
		sellerGroup.DELETE("/ads/:advertisementID", a.SellerHandler.RemoveAdvertisement)
		sellerGroup.POST("/ads/:advertisementID/publish", a.SellerHandler.PublishAdvertisement)
		sellerGroup.POST("/ads/:advertisementID/pause", a.SellerHandler.PauseAdvertisement)
		sellerGroup.POST("/ads/:advertisementID/reactivate", a.SellerHandler.ReactivateAdvertisement)
		sellerGroup.POST("/ads/:advertisementID/images", a.SellerHandler.AddImage)
		sellerGroup.PATCH("/ads/:advertisementID/images/order", a.SellerHandler.ReorderImages)
		sellerGroup.DELETE("/ads/:advertisementID/images/:imageID", a.SellerHandler.RemoveImage)

		sellerGroup.GET("/orders", a.OrdersHandler.ListSellerOrders)
		sellerGroup.GET("/orders/:orderID", a.OrdersHandler.GetSellerOrder)
		sellerGroup.POST("/orders/:orderID/processing", a.OrdersHandler.StartProcessing)
		sellerGroup.POST("/orders/:orderID/shipment", a.OrdersHandler.ShipOrder)
		sellerGroup.POST("/orders/:orderID/documents/:documentID/reject", a.DocumentsHandler.RejectPrescription)

		sellerGroup.GET("/custom-orders", a.CustomOrdersHandler.ListSellerRequests)
		sellerGroup.GET("/custom-orders/:requestID", a.CustomOrdersHandler.GetSellerRequest)
		sellerGroup.POST("/custom-orders/:requestID/review", a.CustomOrdersHandler.ReviewRequest)
		sellerGroup.POST("/custom-orders/:requestID/accept", a.CustomOrdersHandler.AcceptRequest)
		sellerGroup.POST("/custom-orders/:requestID/reject", a.CustomOrdersHandler.RejectRequest)
		sellerGroup.POST("/custom-orders/:requestID/request-info", a.CustomOrdersHandler.RequestInfo)
		sellerGroup.POST("/custom-orders/:requestID/report", a.CustomOrdersHandler.ReportRequest)
		sellerGroup.GET("/custom-orders/:requestID/photo", a.CustomOrdersHandler.OpenSellerPhoto)
	}

	cartGroup := r.Group("/cart", a.AuthMiddleware.Middleware(), a.RoleMiddleware.RequireRole(enum.RoleBuyer))
	{
		cartGroup.GET("", a.OrdersHandler.GetCart)
		cartGroup.POST("/items", a.OrdersHandler.AddCartItem)
		cartGroup.PATCH("/items/:advertisementID", a.OrdersHandler.ChangeCartItemQuantity)
		cartGroup.DELETE("/items/:advertisementID", a.OrdersHandler.RemoveCartItem)
	}

	checkoutGroup := r.Group("/checkout", a.AuthMiddleware.Middleware(), a.RoleMiddleware.RequireRole(enum.RoleBuyer))
	{
		checkoutGroup.POST("", a.OrdersHandler.Checkout)
		checkoutGroup.POST("/preview", a.OrdersHandler.PreviewCheckout)
	}

	ordersGroup := r.Group("/orders", a.AuthMiddleware.Middleware(), a.RoleMiddleware.RequireRole(enum.RoleBuyer))
	{
		ordersGroup.GET("", a.OrdersHandler.ListOrders)
		ordersGroup.GET("/:orderID", a.OrdersHandler.GetOrder)
		ordersGroup.POST("/:orderID/cancel", a.OrdersHandler.CancelOrder)
		ordersGroup.POST("/:orderID/delivered", a.OrdersHandler.ConfirmDelivery)
		ordersGroup.POST("/:orderID/items/:itemID/review", a.ReviewsHandler.CreateReview)
		ordersGroup.POST("/:orderID/items/:itemID/prescription", a.DocumentsHandler.UploadPrescription)
		ordersGroup.GET("/:orderID/payments", a.PaymentsHandler.ListPayments)
		ordersGroup.POST("/:orderID/payments", a.PaymentsHandler.CreatePayment)
	}

	customOrdersGroup := r.Group("/custom-orders", a.AuthMiddleware.Middleware(), a.RoleMiddleware.RequireRole(enum.RoleBuyer))
	{
		customOrdersGroup.GET("", a.CustomOrdersHandler.ListRequests)
		customOrdersGroup.POST("", a.CustomOrdersHandler.CreateRequest)
		customOrdersGroup.GET("/:requestID", a.CustomOrdersHandler.GetRequest)
		customOrdersGroup.POST("/:requestID/complement", a.CustomOrdersHandler.ComplementRequest)
		customOrdersGroup.POST("/:requestID/cancel", a.CustomOrdersHandler.CancelRequest)
		customOrdersGroup.POST("/:requestID/checkout", a.CustomOrdersHandler.Checkout)
		customOrdersGroup.PUT("/:requestID/photo", a.CustomOrdersHandler.SendPhoto)
		customOrdersGroup.GET("/:requestID/photo", a.CustomOrdersHandler.OpenPhoto)
	}

	notificationsGroup := r.Group("/notifications", a.AuthMiddleware.Middleware())
	{
		notificationsGroup.GET("", a.NotificationsHandler.ListNotifications)
		notificationsGroup.POST("/read-all", a.NotificationsHandler.MarkAllRead)
		notificationsGroup.POST("/:notificationID/read", a.NotificationsHandler.MarkRead)
	}

	supportGroup := r.Group("/support", a.AuthMiddleware.Middleware())
	{
		supportGroup.GET("/tickets", a.SupportHandler.ListTickets)
		supportGroup.POST("/tickets", a.SupportHandler.CreateTicket)
		supportGroup.GET("/tickets/:ticketID", a.SupportHandler.GetTicket)
		supportGroup.POST("/tickets/:ticketID/messages", a.SupportHandler.AddMessage)
		supportGroup.GET("/tickets/:ticketID/messages/:messageID/attachment", a.SupportHandler.OpenAttachment)
	}

	adminGroup := r.Group("/admin", a.AuthMiddleware.Middleware(), a.RoleMiddleware.RequireRole(enum.RoleAdmin))
	{
		adminGroup.GET("/indicators", a.AdminHandler.GetIndicators)

		adminGroup.GET("/users", a.AdminHandler.ListUsers)
		adminGroup.POST("/users/:userID/block", a.AdminHandler.BlockUser)
		adminGroup.POST("/users/:userID/suspend", a.AdminHandler.SuspendUser)
		adminGroup.POST("/users/:userID/reactivate", a.AdminHandler.ReactivateUser)

		adminGroup.GET("/ads", a.AdminHandler.ListAds)
		adminGroup.POST("/ads/:advertisementID/remove", a.AdminHandler.RemoveAd)

		adminGroup.GET("/orders", a.AdminHandler.ListOrders)
		adminGroup.GET("/payments", a.AdminHandler.ListPayments)
		adminGroup.GET("/audit-logs", a.AdminHandler.ListAuditLogs)

		adminGroup.GET("/support/tickets", a.SupportHandler.ListQueue)
		adminGroup.POST("/support/tickets/:ticketID/assign", a.SupportHandler.AssignTicket)
		adminGroup.POST("/support/tickets/:ticketID/close", a.SupportHandler.CloseTicket)
	}

	r.POST("/webhooks/asaas", a.PaymentsHandler.AsaasWebhook)

	r.GET("/documents/:documentID", a.AuthMiddleware.Middleware(), a.DocumentsHandler.OpenDocument)

	r.GET("/files/public/*path", a.FilesHandler.ServePublic)
}

func health(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "database unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
