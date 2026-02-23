package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mordenhost/whm2bunny/config"
	"github.com/mordenhost/whm2bunny/internal/bunny"
	"github.com/mordenhost/whm2bunny/internal/notifier"
	"github.com/mordenhost/whm2bunny/internal/provisioner"
	"github.com/mordenhost/whm2bunny/internal/scheduler"
	"github.com/mordenhost/whm2bunny/internal/state"
	"github.com/mordenhost/whm2bunny/internal/webhook"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	// startTime tracks when the server started
	startTime time.Time
	// server holds the HTTP server instance
	server *http.Server
	// provisionerInstance holds the provisioner instance
	provisionerInstance *provisioner.Provisioner
	// stateManager holds the state manager instance
	stateManager *state.Manager
	// telegramNotifier holds the Telegram notifier instance
	telegramNotifier *notifier.TelegramNotifier
	// scheduler holds the scheduler instance
	schedulerInstance *scheduler.Scheduler
	// snapshotStore holds the bandwidth snapshot store
	snapshotStore *state.SnapshotStore
	// bunnyClient holds the Bunny client instance
	bunnyClient *bunny.Client
	// logger holds the logger instance
	logger *zap.Logger
)

// ServeCmd starts the HTTP server to receive webhooks from WHM
var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the webhook HTTP server",
	Long:  "Start the HTTP server that listens for webhooks from WHM/cPanel",
	RunE:  runServe,
}

func init() {
	RootCmd.AddCommand(ServeCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	// 1. Load config
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 2. Initialize logger
	logger, err = initLogger(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("Starting whm2bunny",
		zap.String("version", Version),
		zap.String("commit", Commit),
		zap.String("config", cfgFile),
	)

	startTime = time.Now()

	// 3. Create Bunny client
	bunnyClient = bunny.NewClient(
		cfg.Bunny.APIKey,
		bunny.WithBaseURL(cfg.Bunny.BaseURL),
		bunny.WithLogger(logger),
	)

	// 4. Create state manager
	stateFile := "/var/lib/whm2bunny/state.json"
	if envState := os.Getenv("STATE_FILE"); envState != "" {
		stateFile = envState
	}
	stateManager, err = state.NewManager(stateFile, logger)
	if err != nil {
		return fmt.Errorf("failed to create state manager: %w", err)
	}

	// 5. Create Telegram notifier
	telegramNotifier, err = notifier.NewTelegramNotifier(
		cfg.Telegram.BotToken,
		cfg.Telegram.ChatID,
		cfg.Telegram.Enabled,
		cfg.Telegram.Events,
		logger,
	)
	if err != nil {
		logger.Warn("Failed to initialize Telegram notifier", zap.Error(err))
		// Continue without Telegram
		telegramNotifier = &notifier.TelegramNotifier{}
	}

	// 6. Create provisioner
	provisionerInstance = provisioner.NewProvisioner(
		cfg,
		bunnyClient,
		stateManager,
		telegramNotifier,
		logger,
	)

	// 7. Create webhook handler
	webhookHandler := webhook.NewHandler(
		provisionerInstance,
		cfg.Webhook.Secret,
		logger,
	)

	// 8. Create SnapshotStore and Scheduler
	snapshotFile := "/var/lib/whm2bunny/snapshots.json"
	if envState := os.Getenv("STATE_FILE"); envState != "" && strings.HasSuffix(envState, "state.json") {
		// Use same directory as state file
		snapshotFile = envState[:len(envState)-len("state.json")] + "snapshots.json"
	}
	snapshotStore, err = state.NewSnapshotStore(snapshotFile, logger)
	if err != nil {
		logger.Warn("Failed to create snapshot store", zap.Error(err))
		// Continue without snapshot store
	}

	// Start scheduler if Telegram is enabled
	if telegramNotifier.IsEnabled() {
		schedulerInstance = scheduler.NewScheduler(
			cfg,
			bunnyClient,
			telegramNotifier,
			snapshotStore,
			logger,
		)
		if err := schedulerInstance.Start(); err != nil {
			logger.Warn("Failed to start scheduler", zap.Error(err))
		} else {
			logger.Info("Scheduler started",
				zap.String("daily_schedule", cfg.Telegram.Summary.Schedule),
				zap.String("weekly_schedule", cfg.Telegram.Summary.WeeklySchedule),
			)
		}
	}

	// 9. Start HTTP server with chi router
	if err := startHTTPServer(cfg, webhookHandler); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// 10. Recover pending/failed provisions
	go recoverPendingProvisions()

	// 11. Handle graceful shutdown
	waitForShutdown()

	return nil
}

// initLogger initializes the logger based on config
func initLogger(cfg *config.Config) (*zap.Logger, error) {
	var zapConfig zap.Config

	if cfg.Logging.Format == "json" || verbose {
		zapConfig = zap.NewProductionConfig()
	} else {
		zapConfig = zap.NewDevelopmentConfig()
	}

	// Set log level
	switch cfg.Logging.Level {
	case "debug":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	if verbose {
		zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}

	return zapConfig.Build()
}

// startHTTPServer starts the HTTP server with all routes
func startHTTPServer(cfg *config.Config, webhookHandler *webhook.Handler) error {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Heartbeat("/ping"))

	// Custom middleware for request context
	r.Use(requestContextMiddleware(logger))

	// Routes
	r.Post("/hook", webhookHandler.ServeHTTP)
	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler)

	// Debug routes (only in verbose mode)
	if verbose || os.Getenv("DEBUG") == "true" {
		r.Route("/debug", func(r chi.Router) {
			r.Get("/pending", debugPendingHandler)
			r.Get("/last-error", debugLastErrorHandler)
			r.Post("/retry/{id}", debugRetryHandler)
			r.Get("/state", debugStateHandler)
		})
	}

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server = &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info("HTTP server started",
			zap.String("addr", addr),
			zap.Duration("uptime", time.Since(startTime)),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	return nil
}

// waitForShutdown handles graceful shutdown
func waitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	sig := <-sigChan
	logger.Info("Received shutdown signal", zap.String("signal", sig.String()))

	// Create context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if server != nil {
		logger.Info("Shutting down HTTP server...")
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("HTTP server shutdown error", zap.Error(err))
		}
	}

	// Stop scheduler
	if schedulerInstance != nil {
		logger.Info("Stopping scheduler...")
		schedulerInstance.Stop()
	}

	// Shutdown Telegram notifier
	if telegramNotifier != nil {
		logger.Info("Shutting down Telegram notifier...")
		if err := telegramNotifier.Shutdown(); err != nil {
			logger.Error("Telegram notifier shutdown error", zap.Error(err))
		}
	}

	// Sync logger
	if logger != nil {
		logger.Info("Shutdown complete")
		logger.Sync()
	}
}

// requestContextMiddleware adds request context to each request
func requestContextMiddleware(l *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), "logger", l)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// healthHandler returns the health status of the service
func healthHandler(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(startTime)

	response := map[string]interface{}{
		"status":  "healthy",
		"uptime":  uptime.String(),
		"version": Version,
	}

	respondJSON(w, http.StatusOK, response)
}

// readyHandler checks if the service is ready to accept requests
func readyHandler(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)
	allReady := true

	// Check Bunny API connectivity
	if bunnyClient != nil {
		// Simple connectivity check would go here
		// For now, just mark as ok if client exists
		checks["bunny"] = "ok"
	} else {
		checks["bunny"] = "not initialized"
		allReady = false
	}

	// Check Telegram connectivity
	if telegramNotifier != nil {
		if telegramNotifier.IsEnabled() {
			checks["telegram"] = "ok"
		} else {
			checks["telegram"] = "disabled"
		}
	} else {
		checks["telegram"] = "not initialized"
	}

	// Check state manager
	if stateManager != nil {
		checks["state"] = "ok"
	} else {
		checks["state"] = "not initialized"
		allReady = false
	}

	statusCode := http.StatusOK
	if !allReady {
		statusCode = http.StatusServiceUnavailable
	}

	response := map[string]interface{}{
		"ready":  allReady,
		"checks": checks,
	}

	respondJSON(w, statusCode, response)
}

// debugPendingHandler lists pending provisioning operations
func debugPendingHandler(w http.ResponseWriter, r *http.Request) {
	if stateManager == nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "state manager not initialized",
		})
		return
	}

	pending := stateManager.ListPending()

	response := map[string]interface{}{
		"count":  len(pending),
		"states": pending,
	}

	respondJSON(w, http.StatusOK, response)
}

// debugLastErrorHandler returns the last 10 errors with details
func debugLastErrorHandler(w http.ResponseWriter, r *http.Request) {
	if stateManager == nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "state manager not initialized",
		})
		return
	}

	failed := stateManager.ListFailed()

	// Limit to last 10 errors
	count := len(failed)
	if count > 10 {
		failed = failed[len(failed)-10:]
	}

	response := map[string]interface{}{
		"total_failed": count,
		"showing":      len(failed),
		"errors":       failed,
	}

	respondJSON(w, http.StatusOK, response)
}

// debugRetryHandler retries a failed provisioning operation
func debugRetryHandler(w http.ResponseWriter, r *http.Request) {
	if stateManager == nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "state manager not initialized",
		})
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "id parameter is required",
		})
		return
	}

	// Get the state
	st, err := stateManager.Get(id)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{
			"error": "state not found",
		})
		return
	}

	// Check if it's failed
	if st.Status != state.StatusFailed {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "state is not in failed status",
		})
		return
	}

	// Reset to pending for retry
	st.Status = state.StatusPending
	if err := stateManager.Update(st); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to update state",
		})
		return
	}

	// Trigger retry in background
	go func() {
		if provisionerInstance != nil {
			logger.Info("Triggering retry", zap.String("id", id), zap.String("domain", st.Domain))
			if err := provisionerInstance.Provision(st.Domain, ""); err != nil {
				logger.Error("Retry failed",
					zap.String("id", id),
					zap.String("domain", st.Domain),
					zap.Error(err),
				)
			} else {
				logger.Info("Retry succeeded",
					zap.String("id", id),
					zap.String("domain", st.Domain),
				)
			}
		}
	}()

	response := map[string]interface{}{
		"message": "retry scheduled",
		"id":      id,
		"domain":  st.Domain,
	}

	respondJSON(w, http.StatusAccepted, response)
}

// debugStateHandler returns all states
func debugStateHandler(w http.ResponseWriter, r *http.Request) {
	if stateManager == nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "state manager not initialized",
		})
		return
	}

	states := stateManager.ListAll()

	response := map[string]interface{}{
		"total":  len(states),
		"states": states,
	}

	respondJSON(w, http.StatusOK, response)
}

// recoverPendingProvisions recovers pending/failed provisions on startup
// Runs in background with backoff delay between each domain
func recoverPendingProvisions() {
	if provisionerInstance == nil || stateManager == nil {
		return
	}

	// Wait a few seconds after server starts before recovery
	time.Sleep(5 * time.Second)

	pendingCount := len(stateManager.Recover())
	if pendingCount == 0 {
		logger.Info("No pending/failed provisions to recover")
		return
	}

	logger.Info("Starting recovery of pending/failed provisions",
		zap.Int("count", pendingCount),
	)

	ctx := context.Background()
	if err := provisionerInstance.Recover(ctx); err != nil {
		logger.Error("Recovery failed", zap.Error(err))
	}
}

// respondJSON writes a JSON response
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	// Use json.Marshal instead of encoder for simpler error handling
	if data != nil {
		// In production, you'd want proper error handling here
		// For now, we'll just write the JSON
		buf, _ := json.Marshal(data)
		w.Write(buf)
	}
}
