package bot

import (
	"log"
	"sync"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/commands"
	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/bwmarrin/discordgo"
)

// Bot represents the Discord bot
type Bot struct {
	session          *discordgo.Session
	db               *database.Queries
	router           *commands.Router
	activeSessions   map[string]time.Time // Maps user_id to session start time
	activeSessionMu  sync.Mutex
	LoggingChannelID string // Added to store the logging channel ID
}

// New creates a new Discord bot instance
func New(token string, db *database.Queries, loggingChannelID string) (*Bot, error) {
	// Create a new Discord session
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	// Create command router
	router := commands.NewRouter(db, "!")

	// Register commands
	commands.RegisterTimeTrackingCommands(router)

	bot := &Bot{
		session:          dg,
		db:               db,
		router:           router,
		activeSessions:   make(map[string]time.Time),
		LoggingChannelID: loggingChannelID, // Store the logging channel ID
	}

	// Register handlers
	dg.AddHandler(bot.handleReady)
	dg.AddHandler(bot.handleVoiceStateUpdate)
	dg.AddHandler(bot.handleMessageCreate)

	// We only care about voice and guild messages
	dg.Identify.Intents = discordgo.IntentsGuildVoiceStates | discordgo.IntentsGuildMessages

	// Open the websocket and begin listening
	err = dg.Open()
	if err != nil {
		return nil, err
	}

	return bot, nil
}

// Close closes the Discord session
func (b *Bot) Close() {
	// End all active sessions before shutting down
	b.endAllActiveSessions()
	b.session.Close()
}

// endAllActiveSessions ends all active study sessions when the bot shuts down
func (b *Bot) endAllActiveSessions() {
	b.activeSessionMu.Lock()
	defer b.activeSessionMu.Unlock()

	now := time.Now()
	for userID, startTime := range b.activeSessions {
		log.Printf("Ending session for user %s on bot shutdown", userID)

		// End the session in the DB
		// Note: We don't have the session ID here, so we'd need to implement
		// a different query to find and end the active session by user ID
		duration := now.Sub(startTime)
		durationMs := duration.Milliseconds()

		// This is a placeholder - in a real implementation, you would:
		// 1. Find the active session ID for this user
		// 2. End that specific session
		// 3. Update user stats
		log.Printf("User %s studied for %v (%d ms)", userID, duration, durationMs)
	}
}

// handleMessageCreate is called when a message is created in a channel
func (b *Bot) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Use the router to handle commands
	b.router.HandleMessage(s, m)
}
