package bot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/bwmarrin/discordgo"
)

// Bot represents the Discord bot
type Bot struct {
	session                *discordgo.Session
	db                     *database.Queries
	activeSessions         map[string]time.Time // Maps user_id to session start time
	activeSessionMu        sync.Mutex
	LoggingChannelID       string              // Added to store the logging channel ID
	testGuildID            string              // Added to store the test guild ID for command registration
	allowedVoiceChannelIDs map[string]struct{} // For storing allowed voice channel IDs

	// Slash command specific fields
	commandHandlers map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)
}

// New creates a new Discord bot instance
func New(token string, db *database.Queries, loggingChannelID string, testGuildID string, allowedVCs map[string]struct{}) (*Bot, error) {
	// Create a new Discord session
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		session:                dg,
		db:                     db,
		activeSessions:         make(map[string]time.Time),
		LoggingChannelID:       loggingChannelID, // Store the logging channel ID
		testGuildID:            testGuildID,      // Store the test guild ID
		allowedVoiceChannelIDs: allowedVCs,       // Store the allowed voice channels map
		commandHandlers:        make(map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate)),
	}

	// Register slash command handlers here
	bot.commandHandlers["stats"] = bot.handleSlashStatsCommand
	bot.commandHandlers["leaderboard"] = bot.handleSlashLeaderboardCommand
	bot.commandHandlers["help"] = bot.handleSlashHelpCommand

	// Register handlers
	dg.AddHandler(bot.handleReady)
	dg.AddHandler(bot.handleVoiceStateUpdate)
	dg.AddHandler(bot.handleInteractionCreate)

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

		// If LoggingChannelID is set, also send a message about the shutdown-ended session
		if b.LoggingChannelID != "" {
			username := userID // We don't have the username directly here, ideally fetch it or log UserID
			// Consider fetching username if this message is important. For now, using UserID.
			// Potentially fetch from DB if user was created, or make a Discord API call (expensive on shutdown)

			// Attempt to get user object from Discord state (might not always be available)
			discordUser, err := b.session.User(userID)
			if err == nil && discordUser != nil {
				username = discordUser.Username
			}

			message := fmt.Sprintf("<@%s> (%s) session ended due to bot shutdown after %s.", userID, username, formatDuration(duration))
			_, err = b.session.ChannelMessageSend(b.LoggingChannelID, message)
			if err != nil {
				log.Printf("Error sending shutdown session message to Discord channel %s: %v", b.LoggingChannelID, err)
			}
		}
	}
}

func (b *Bot) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)

	guildID := b.testGuildID // Use the configured testGuildID

	if guildID == "" {
		log.Println("Registering GLOBAL slash commands...")
	} else {
		log.Printf("Registering slash commands for TEST guild ID: %s", guildID)
	}

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "stats",
			Description: "Shows your study/voice channel time statistics.",
		},
		{
			Name:        "leaderboard",
			Description: "Shows the study time leaderboard.",
		},
		{
			Name:        "help",
			Description: "Shows available commands and information about the bot.",
		},
	}

	// Iterate and register commands
	// Note: For global commands, it can take up to an hour for them to propagate.
	// For guild-specific commands (faster registration for testing), you use:
	// s.ApplicationCommandCreate(s.State.User.ID, "YOUR_GUILD_ID", cmd)
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, cmd := range commands {
		regCmd, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, cmd) // Using configured guildID
		if err != nil {
			log.Printf("Cannot create '%v' command in guild %s: %v", cmd.Name, guildID, err)
		} else {
			registeredCommands[i] = regCmd
			log.Printf("Registered command: %s", regCmd.Name)
		}
	}
}

func (b *Bot) handleInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type == discordgo.InteractionApplicationCommand {
		if handler, ok := b.commandHandlers[i.ApplicationCommandData().Name]; ok {
			handler(s, i)
		} else {
			log.Printf("Unknown command received: %s", i.ApplicationCommandData().Name)
			// Optionally send an ephemeral message back to the user
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Unknown command.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
		}
	}
}

// handleSlashStatsCommand is the new handler for the /stats slash command
func (b *Bot) handleSlashStatsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background() // Or derive from a base context if you have one

	// User ID from interaction
	userID := ""
	if i.Member != nil && i.Member.User != nil { // Interaction in a Guild
		userID = i.Member.User.ID
	} else if i.User != nil { // Interaction in DMs
		userID = i.User.ID
	}

	if userID == "" {
		log.Println("Error: could not determine UserID from interaction for /stats command")
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Error: Could not identify user.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Username for display
	username := ""
	if i.Member != nil && i.Member.User != nil {
		username = i.Member.User.Username
	} else if i.User != nil {
		username = i.User.Username
	}

	// Check if user exists
	_, err := b.db.GetUser(ctx, userID)
	if err != nil {
		// Create the user if they don't exist
		_, createErr := b.db.CreateUser(ctx, database.CreateUserParams{
			UserID:   userID,
			Username: sql.NullString{String: username, Valid: true},
		})
		if createErr != nil {
			log.Printf("Error creating user via slash command: %v", createErr)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Error creating your user profile. Please try again or join a voice channel first.",
					Flags:   discordgo.MessageFlagsEphemeral,
				},
			})
			return
		}
	}

	// Get user stats
	stats, err := b.db.GetUserStats(ctx, userID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You haven't studied yet! Join a voice channel to start tracking your study time.",
				// No Ephemeral flag here, so message is visible
			},
		})
		return
	}

	// Format the study times
	var totalMs, dailyMs, weeklyMs, monthlyMs int64
	if stats.TotalStudyMs.Valid {
		totalMs = stats.TotalStudyMs.Int64
	}
	if stats.DailyStudyMs.Valid {
		dailyMs = stats.DailyStudyMs.Int64
	}
	if stats.WeeklyStudyMs.Valid {
		weeklyMs = stats.WeeklyStudyMs.Int64
	}
	if stats.MonthlyStudyMs.Valid {
		monthlyMs = stats.MonthlyStudyMs.Int64
	}

	total := time.Duration(totalMs) * time.Millisecond
	daily := time.Duration(dailyMs) * time.Millisecond
	weekly := time.Duration(weeklyMs) * time.Millisecond
	monthly := time.Duration(monthlyMs) * time.Millisecond

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("Study Stats for %s", username),
		Description: "Your study time statistics from voice channels.",
		Color:       0x00AAFF, // Blue color
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Total Study Time", Value: formatDuration(total), Inline: true},
			{Name: "Today", Value: formatDuration(daily), Inline: true},
			{Name: "This Week", Value: formatDuration(weekly), Inline: true},
			{Name: "This Month", Value: formatDuration(monthly), Inline: true},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer:    &discordgo.MessageEmbedFooter{Text: "Keep up the good work!"},
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
	if err != nil {
		log.Printf("Error sending slash command response for /stats: %v", err)
	}
}

// handleSlashLeaderboardCommand handles the /leaderboard slash command
func (b *Bot) handleSlashLeaderboardCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()

	leaderboardData, err := b.db.GetLeaderboard(ctx)
	if err != nil {
		log.Printf("Error fetching leaderboard data: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Error: Could not fetch leaderboard data at this time. Please try again later.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if len(leaderboardData) == 0 {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No one is on the leaderboard yet! Start studying to get your name up here.",
			},
		})
		return
	}

	embedFields := []*discordgo.MessageEmbedField{}
	for rank, entry := range leaderboardData {
		// entry.Username is sql.NullString, entry.TotalStudyMs is sql.NullInt64
		// entry.UserID is string (not nullable in users table schema, assuming)
		username := "Unknown User"
		if entry.Username.Valid {
			username = entry.Username.String
		}

		durationMs := int64(0)
		if entry.TotalStudyMs.Valid {
			durationMs = entry.TotalStudyMs.Int64
		}
		duration := time.Duration(durationMs) * time.Millisecond

		embedFields = append(embedFields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%d. %s", rank+1, username),
			Value:  fmt.Sprintf("Time Studied: %s (<@%s>)", formatDuration(duration), entry.UserID),
			Inline: false,
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🏆 Study Time Leaderboard - Top 10",
		Description: "See who has been putting in the hours!",
		Color:       0xFFD700, // Gold color
		Fields:      embedFields,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "LockIn Bot Leaderboard"},
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
	if err != nil {
		log.Printf("Error sending slash command response for /leaderboard: %v", err)
	}
}

// handleSlashHelpCommand handles the /help slash command
func (b *Bot) handleSlashHelpCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "LockIn Bot Help",
		Description: "Hi there! I'm LockIn Bot. I track time spent in voice channels and help you stay focused.",
		Color:       0x00AAFF, // Blue color
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "`/stats`",
				Value: "Shows your personal voice channel time statistics (total, today, this week, this month).",
			},
			{
				Name:  "`/leaderboard`",
				Value: "Displays the top users by voice channel time (Coming Soon!).",
			},
			{
				Name:  "`/help`",
				Value: "Shows this help message.",
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer:    &discordgo.MessageEmbedFooter{Text: "LockIn Bot"},
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			// Flags:   discordgo.MessageFlagsEphemeral, // Optional: make it visible only to the user
		},
	})
	if err != nil {
		log.Printf("Error sending slash command response for /help: %v", err)
	}
}

// formatDuration is defined in handlers.go within the same package
// // formatDuration (copied from internal/commands/time_tracking.go for now, can be moved to a common utils package)
// func formatDuration(d time.Duration) string {
// 	d = d.Round(time.Second)
// 	h := d / time.Hour
// 	d -= h * time.Hour
// 	m := d / time.Minute
// 	d -= m * time.Minute
// 	s := d / time.Second
//
// 	if h > 0 {
// 		return fmt.Sprintf("%dh %dm %ds", h, m, s)
// 	}
// 	if m > 0 {
// 		return fmt.Sprintf("%dm %ds", m, s)
// 	}
// 	return fmt.Sprintf("%ds", s)
// }

// handleVoiceStateUpdate is called when a user's voice state changes
func (b *Bot) handleVoiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	// Get the user
	user, err := s.User(v.UserID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		return
	}

	// Ignore bot events
	if user.Bot {
		return
	}

	// Check if specific voice channels are configured for tracking
	trackSpecificChannels := len(b.allowedVoiceChannelIDs) > 0

	if v.ChannelID != "" { // User is in some voice channel
		isAllowedChannel := true // Assume allowed if not configured to restrict
		if trackSpecificChannels {
			_, isAllowedChannel = b.allowedVoiceChannelIDs[v.ChannelID]
		}

		if isAllowedChannel {
			// User is in an allowed channel (or all channels are allowed)
			b.handleUserJoinedVoice(s, v, user)
		} else {
			// User is in a non-allowed channel, ensure any existing session is ended
			log.Printf("User %s (%s) joined non-tracked voice channel %s. Ending any active session.", user.Username, v.UserID, v.ChannelID)
			b.handleUserLeftVoice(s, v, user) // Treat as if they left a tracked session context
		}
	} else { // User left all voice channels
		// User left all voice channels, end their session as usual
		b.handleUserLeftVoice(s, v, user)
	}
}

// handleUserJoinedVoice processes a user joining a voice channel
// ... existing code (no changes needed here as the decision to call it is now made in handleVoiceStateUpdate) ...

// handleUserLeftVoice processes a user leaving a voice channel
// ... existing code (no changes needed here) ...
