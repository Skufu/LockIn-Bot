package commands

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/bwmarrin/discordgo"
)

// DBContextKey is the key used to store the database in the context
type DBContextKey string

const (
	// DBKey is the key used to store the database in the context
	DBKey DBContextKey = "db"
)

// RegisterTimeTrackingCommands registers time tracking commands with the router
func RegisterTimeTrackingCommands(router *Router) {
	router.Register("study", "Shows your study statistics", handleStudyCommand)
	router.Register("leaderboard", "Shows the study time leaderboard", handleLeaderboardCommand)
	router.Register("help", "Shows available commands", handleHelpCommand)
}

// handleStudyCommand handles the study command
func handleStudyCommand(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	db := getDBFromContext(ctx)
	if db == nil {
		s.ChannelMessageSend(m.ChannelID, "Error: Database connection not available")
		return
	}

	// Check if user exists
	_, err := db.GetUser(ctx, m.Author.ID)
	if err != nil {
		// Create the user if they don't exist
		_, err = db.CreateUser(ctx, database.CreateUserParams{
			UserID:   m.Author.ID,
			Username: sql.NullString{String: m.Author.Username, Valid: true},
		})
		if err != nil {
			log.Printf("Error creating user: %v", err)
			s.ChannelMessageSend(m.ChannelID, "Error creating user profile. Please try again.")
			return
		}
	}

	// Get user stats
	stats, err := db.GetUserStats(ctx, m.Author.ID)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "You haven't studied yet! Join a voice channel to start tracking your study time.")
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

	// Create embed message
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("Study Stats for %s", m.Author.Username),
		Description: "Your study time statistics",
		Color:       0x00AAFF, // Blue color
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Total Study Time",
				Value:  formatDuration(total),
				Inline: true,
			},
			{
				Name:   "Today",
				Value:  formatDuration(daily),
				Inline: true,
			},
			{
				Name:   "This Week",
				Value:  formatDuration(weekly),
				Inline: true,
			},
			{
				Name:   "This Month",
				Value:  formatDuration(monthly),
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Keep up the good work!",
		},
	}

	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}

// handleLeaderboardCommand handles the leaderboard command
func handleLeaderboardCommand(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	// This is a placeholder for the leaderboard command
	// In a complete implementation, this would query the database for top users
	// For now, we'll just show a message
	s.ChannelMessageSend(m.ChannelID, "Leaderboard feature coming soon!")
}

// handleHelpCommand handles the help command
func handleHelpCommand(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	// Get the router from context
	router, ok := ctx.Value("router").(*Router)
	if !ok || router == nil {
		s.ChannelMessageSend(m.ChannelID, "Available commands: !study, !leaderboard, !help")
		return
	}

	// Get help text from router
	helpText := router.GetHelpText()
	s.ChannelMessageSend(m.ChannelID, helpText)
}

// Helper function to get database from context
func getDBFromContext(ctx context.Context) *database.Queries {
	if db, ok := ctx.Value(DBKey).(*database.Queries); ok {
		return db
	}
	return nil
}

// formatDuration converts a time.Duration to a human-readable string
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
