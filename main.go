// Kiro Gateway - Go Implementation
// A proxy gateway for Kiro API (Amazon Q Developer / AWS CodeWhisperer)
// Providing OpenAI and Anthropic compatible interfaces
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kiro-go-proxy/api"
	"kiro-go-proxy/auth"
	"kiro-go-proxy/config"
	"kiro-go-proxy/model"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func main() {
	// Parse command line arguments
	host := flag.String("host", "", "Server host address")
	port := flag.Int("port", 0, "Server port")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Kiro Gateway v%s\n", config.AppVersion)
		os.Exit(0)
	}

	// Load configuration
	cfg := config.Load()

	// Override with CLI arguments
	if *host != "" {
		cfg.ServerHost = *host
	}
	if *port != 0 {
		cfg.ServerPort = *port
	}

	// Setup logging
	setupLogging(cfg.LogLevel)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Print startup banner
	printBanner(cfg.ServerHost, cfg.ServerPort)

	// Initialize authentication manager
	authManager := auth.NewManager(cfg)

	// Create API server
	server := api.NewServer(cfg, authManager)

	// Load models from Kiro API
	loadModels(server, authManager, cfg)

	// Setup Gin router
	if cfg.LogLevel == "DEBUG" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	// Setup routes
	server.SetupRoutes(router)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.StreamingReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.StreamingReadTimeout) * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Infof("Starting server on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Errorf("Server shutdown error: %v", err)
	}

	log.Info("Server stopped")
}

func setupLogging(level string) {
	switch level {
	case "DEBUG":
		log.SetLevel(log.DebugLevel)
	case "WARNING":
		log.SetLevel(log.WarnLevel)
	case "ERROR":
		log.SetLevel(log.ErrorLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
}

func printBanner(host string, port int) {
	displayHost := host
	if host == "0.0.0.0" {
		displayHost = "localhost"
	}

	fmt.Println()
	fmt.Printf("  👻 Kiro Gateway v%s\n", config.AppVersion)
	fmt.Println()
	fmt.Println("  Server running at:")
	fmt.Printf("  ➜  http://%s:%d\n", displayHost, port)
	fmt.Println()
	fmt.Printf("  API Docs:      http://%s:%d/docs\n", displayHost, port)
	fmt.Printf("  Health Check:  http://%s:%d/health\n", displayHost, port)
	fmt.Println()
	fmt.Println("  ────────────────────────────────────────────────────────")
	fmt.Println("  💬 Found a bug? Need help? Have questions?")
	fmt.Println("  ➜  https://github.com/lorne-luo/kiro-go-proxy/issues")
	fmt.Println("  ────────────────────────────────────────────────────────")
	fmt.Println()
}

func loadModels(server *api.Server, authManager *auth.Manager, cfg *config.Config) {
	// Try to fetch models from Kiro API
	token, err := authManager.GetAccessToken()
	if err != nil {
		log.Warnf("Failed to get token for model list: %v", err)
		return
	}

	// Create temporary model cache for loading
	modelCache := model.NewCache(cfg)

	// Build request
	url := fmt.Sprintf("%s/ListAvailableModels?origin=AI_EDITOR", authManager.QHost())
	if authManager.ProfileArn() != "" {
		url += "&profileArn=" + authManager.ProfileArn()
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Warnf("Failed to create model list request: %v", err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Warnf("Failed to fetch models from Kiro API: %v", err)
		log.Warn("Using fallback model list")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warnf("Model list request failed with status %d", resp.StatusCode)
		log.Warn("Using fallback model list")
		return
	}

	var result struct {
		Models []model.Info `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Warnf("Failed to parse model list: %v", err)
		return
	}

	modelCache.Update(result.Models)
	log.Infof("Loaded %d models from Kiro API", len(result.Models))

	// Update server's cache with loaded models
	for _, m := range modelCache.GetAllModelIDs() {
		server.ModelCache.AddHiddenModel(m, m)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Requested-With, Accept")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

