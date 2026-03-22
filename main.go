package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/bot"
	"github.com/Skufu/LockIn-Bot/internal/config"
	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/Skufu/LockIn-Bot/internal/service"
)

const maxRetries = 10

var (
	lastHealthCheckLogMu   sync.Mutex
	lastHealthCheckLog     time.Time
	healthCheckLogInterval = 5 * time.Minute
	botReady               bool
	botReadyMu             sync.Mutex
)

func main() {
	// Load configuration
	log.Println("Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	log.Printf("Connecting to Neon PostgreSQL database at %s...", cfg.DBHost)
	startTime := time.Now()

	db, err := database.Connect(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	log.Printf("Successfully connected to database in %v", time.Since(startTime))

	// Run migrations
	log.Println("Running database migrations...")
	err = db.MigrateUp("db/migrations")
	if err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations completed successfully")

	// Start the HTTP health check server FIRST
	// This prevents Render from killing the process for not binding a port,
	// which would cause a restart loop that triggers Cloudflare rate limits.
	startHealthCheckServer()

	// Log token diagnostics (masked) to help debug configuration issues on deploy
	tokenLen := len(cfg.DiscordToken)
	tokenPreview := cfg.DiscordToken
	if tokenLen > 8 {
		tokenPreview = cfg.DiscordToken[:8]
	}
	log.Printf("Token diagnostics: length=%d, prefix=%s...", tokenLen, tokenPreview)

	// Add an initial delay before connecting to Discord to let any Cloudflare rate limits expire
	// This is critical when Render restarts the process — without this delay, rapid restarts
	// trigger Cloudflare error 1015 (CDN-level rate limiting on shared IPs)
	// INCREASED: 30 seconds to give Cloudflare blocks more time to expire
	log.Println("Waiting 30 seconds before connecting to Discord (avoiding Cloudflare rate limits)...")
	time.Sleep(30 * time.Second)

	// Create and start the bot with retry logic
	log.Println("Initializing Discord bot with retry logic...")
	log.Printf("Discord connection retry enabled: max %d attempts, exponential backoff", maxRetries)

	discordBot, err := bot.ConnectWithRetry(cfg.DiscordToken, db.Querier, cfg, cfg.AllowedVoiceChannelIDsMap, maxRetries)
	if err != nil {
		// Check if it's a permanent error vs all retries exhausted
		permanentError, isBotStartupError := err.(bot.BotStartupError)
		if isBotStartupError && permanentError.Type == bot.ErrorTypePermanent {
			log.Fatalf("Failed to initialize Discord bot with permanent error (cannot retry): %v", err)
		} else {
			log.Fatalf("Failed to initialize Discord bot after %d attempts (all retries exhausted): %v", maxRetries, err)
		}
	}

	// Mark bot as ready
	botReadyMu.Lock()
	botReady = true
	botReadyMu.Unlock()

	// Start connection monitoring (but don't auto-shutdown on token errors)
	discordBot.MonitorConnection()

	// Initialize StreakService
	log.Println("Initializing Streak Service...")
	streakService := service.NewStreakService(db.Querier, discordBot.Session(), cfg)

	// SET the StreakService on the Bot instance
	discordBot.SetStreakService(streakService)

	// SET the Bot reference on StreakService to access session timing
	streakService.SetBot(discordBot)

	// Start StreakService scheduled tasks (can be after setting it on the bot)
	streakService.StartScheduledTasks()

	// Initialize AchievementService
	log.Println("Initializing Achievement Service...")
	achievementService := service.NewAchievementService(db.Querier, discordBot.Session(), cfg)
	discordBot.SetAchievementService(achievementService)

	// Connect AchievementService to StreakService for streak-based achievements
	streakService.SetAchievementService(achievementService)

	// Create and start the scheduler for existing bot tasks (e.g., study session resets)
	scheduler := bot.NewScheduler(discordBot)
	scheduler.Start()

	// Wait for a CTRL-C
	log.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	receivedSignal := <-sc

	// Log what signal caused the shutdown
	log.Printf("Received signal: %v", receivedSignal)
	log.Println("Shutting down...")
	scheduler.Stop()
	streakService.StopScheduledTasks()
	discordBot.Close()
	log.Println("Shutdown complete. Goodbye!")
}

// startHealthCheckServer starts the HTTP health check server
// If PORT is set (Web Service), it binds to the port to satisfy Render.
// If PORT is not set (Background Worker), it gracefully skips starting the server.
func startHealthCheckServer() {
	port := os.Getenv("PORT")
	if port == "" {
		log.Println("PORT environment variable not set. Skipping health check server (running as Background Worker).")
		return
	}

	log.Printf("Starting health check server on port %s (from PORT env var)", port)

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		lastHealthCheckLogMu.Lock()
		shouldLog := lastHealthCheckLog.IsZero() || time.Since(lastHealthCheckLog) >= healthCheckLogInterval
		if shouldLog {
			lastHealthCheckLog = time.Now()
		}
		lastHealthCheckLogMu.Unlock()

		if shouldLog {
			log.Printf("Health check request received: %s %s", r.Method, r.URL.Path)
		}

		botReadyMu.Lock()
		ready := botReady
		botReadyMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if ready {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy","service":"lockin-bot","discord":"connected"}`))
		} else {
			w.WriteHeader(http.StatusOK) // Still return 200 so Render doesn't restart us
			w.Write([]byte(`{"status":"starting","service":"lockin-bot","discord":"connecting"}`))
		}
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","service":"lockin-bot","message":"LockIn Bot is running"}`))
	})

	go func() {
		log.Printf("Health check server listening on :%s", port)
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			log.Printf("Warning: Health check server failed to start: %v", err)
		}
	}()

	// Give the HTTP server a moment to bind the port
	time.Sleep(500 * time.Millisecond)
	log.Println("Health check server started successfully")
}
