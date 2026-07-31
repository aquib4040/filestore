package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"filestore/pkg/config"
	"filestore/pkg/db"
	"filestore/pkg/telegram"

	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	logger.Info("Starting Go FileStore Bot...")

	// 1. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	if cfg.BotToken == "" || cfg.MongoURI == "" {
		logger.Fatal("BOT_TOKEN and MONGO_URI environment variables must be set!")
	}

	// 2. Connect to MongoDB
	mongoDB, err := db.NewMongoDB(cfg.MongoURI, cfg.DBName, cfg.TokenCryptKey)
	if err != nil {
		logger.Fatal("Failed to connect to MongoDB", zap.Error(err))
	}
	logger.Info("Connected to MongoDB successfully")

	// 3. Initialize Bot Manager
	manager := telegram.NewBotManager(cfg, mongoDB, logger)

	// Create root context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4. Start Bot Manager
	err = manager.Start(ctx)
	if err != nil {
		logger.Fatal("Failed to start bot manager", zap.Error(err))
	}

	// 5. Start HTTP health check listener (important for Koyeb/Render health check binds)
	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "OK - FileStore Bot is alive and running!")
		})
		logger.Info("Starting HTTP health check server", zap.String("port", cfg.Port))
		if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
			logger.Warn("HTTP server closed or failed to start", zap.Error(err))
		}
	}()

	// 5.5 Start HTTP self-ping worker if FQDN is configured (keeps free tiers awake)
	if cfg.FQDN != "" {
		go func() {
			url := cfg.FQDN
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				url = "https://" + url
			}
			logger.Info("Starting self-ping worker", zap.String("url", url))

			ticker := time.NewTicker(3 * time.Minute)
			defer ticker.Stop()

			client := &http.Client{Timeout: 15 * time.Second}
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					resp, err := client.Get(url)
					if err == nil {
						resp.Body.Close()
						logger.Debug("Self-ping check successful", zap.String("url", url), zap.Int("status", resp.StatusCode))
					} else {
						logger.Warn("Self-ping check failed", zap.String("url", url), zap.Error(err))
					}
				}
			}
		}()
	}

	// 6. Graceful Shutdown handler
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigs:
		logger.Info("Received termination signal, shutting down gracefully...", zap.String("signal", sig.String()))
		cancel()
	case <-ctx.Done():
		logger.Info("Context cancelled, shutting down...")
	}

	logger.Info("All bot services stopped. Goodbye!")
}
