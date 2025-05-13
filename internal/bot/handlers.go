package bot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/bwmarrin/discordgo"
)

// handleReady is called when the bot connects to Discord
func (b *Bot) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
}

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

	// Determine if user joined or left a voice channel
	if v.ChannelID != "" {
		// User joined or moved to a voice channel
		b.handleUserJoinedVoice(s, v, user)
	} else {
		// User left a voice channel
		b.handleUserLeftVoice(s, v, user)
	}
}

// handleUserJoinedVoice processes a user joining a voice channel
func (b *Bot) handleUserJoinedVoice(s *discordgo.Session, v *discordgo.VoiceStateUpdate, user *discordgo.User) {
	ctx := context.Background()

	b.activeSessionMu.Lock()
	defer b.activeSessionMu.Unlock()

	// Check if user already has an active session
	if _, exists := b.activeSessions[v.UserID]; exists {
		// User was already in a voice channel and moved to another one
		// For now, we'll just continue their existing session
		return
	}

	// User joined a voice channel
	now := time.Now()
	b.activeSessions[v.UserID] = now

	// Create DB user if they don't exist
	_, err := b.db.CreateUser(ctx, database.CreateUserParams{
		UserID:   v.UserID,
		Username: sql.NullString{String: user.Username, Valid: true},
	})
	if err != nil {
		log.Printf("Error creating user: %v", err)
	}

	// Create a new study session
	session, err := b.db.CreateStudySession(ctx, database.CreateStudySessionParams{
		UserID:    sql.NullString{String: v.UserID, Valid: true},
		StartTime: now,
	})
	if err != nil {
		log.Printf("Error creating study session: %v", err)
	} else {
		log.Printf("Started session %d for user %s", session.SessionID, v.UserID)
	}
}

// handleUserLeftVoice processes a user leaving a voice channel
func (b *Bot) handleUserLeftVoice(s *discordgo.Session, v *discordgo.VoiceStateUpdate, user *discordgo.User) {
	ctx := context.Background()

	b.activeSessionMu.Lock()
	defer b.activeSessionMu.Unlock()

	// Check if user had an active session
	_, exists := b.activeSessions[v.UserID]
	if !exists {
		// No active session to end
		return
	}

	// User left a voice channel, end their study session
	now := time.Now()
	delete(b.activeSessions, v.UserID)

	// Find the active session in the database
	activeSession, err := b.db.GetActiveStudySession(ctx, sql.NullString{String: v.UserID, Valid: true})
	if err != nil {
		log.Printf("Error getting active session: %v", err)
		return
	}

	// End the session
	session, err := b.db.EndStudySession(ctx, database.EndStudySessionParams{
		SessionID: activeSession.SessionID,
		EndTime:   sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		log.Printf("Error ending study session: %v", err)
		return
	}

	// Update user stats
	if session.DurationMs.Valid {
		durationMs := session.DurationMs.Int64
		_, err = b.db.CreateOrUpdateUserStats(ctx, database.CreateOrUpdateUserStatsParams{
			UserID:       v.UserID,
			TotalStudyMs: sql.NullInt64{Int64: durationMs, Valid: true},
		})
		if err != nil {
			log.Printf("Error updating user stats: %v", err)
		}

		duration := time.Duration(durationMs) * time.Millisecond
		log.Printf("User %s studied for %v (%d ms)", v.UserID, duration, durationMs)
	}
}

// handleStudyCommand processes the !study command to show study stats
// This is kept for backward compatibility but will be replaced by the command router
func (b *Bot) handleStudyCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	ctx := context.Background()

	// Check if user exists
	_, err := b.db.GetUser(ctx, m.Author.ID)
	if err != nil {
		// Create the user if they don't exist
		_, err = b.db.CreateUser(ctx, database.CreateUserParams{
			UserID:   m.Author.ID,
			Username: sql.NullString{String: m.Author.Username, Valid: true},
		})
		if err != nil {
			log.Printf("Error creating user: %v", err)
			return
		}
	}

	// Get user stats
	stats, err := b.db.GetUserStats(ctx, m.Author.ID)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "You haven't studied yet!")
		return
	}

	// Format the study times (safely handling nullable fields)
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

	// Create a formatted message
	message := fmt.Sprintf("**Study Stats for %s**\n"+
		"Total: %s\n"+
		"Today: %s\n"+
		"This week: %s\n"+
		"This month: %s\n",
		m.Author.Username,
		formatDuration(total),
		formatDuration(daily),
		formatDuration(weekly),
		formatDuration(monthly))

	s.ChannelMessageSend(m.ChannelID, message)
}

// formatDuration converts a time.Duration to a human-readable string
// e.g., "2h 15m 30s" or "45m 20s"
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
