package commands

import (
	"context"
	"log"
	"time"

	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/bwmarrin/discordgo"
)

// AdminCommands handles administrative commands
type AdminCommands struct {
	db *database.Queries
}

// NewAdminCommands creates a new AdminCommands instance
func NewAdminCommands(db *database.Queries) *AdminCommands {
	return &AdminCommands{db: db}
}

// HandleCleanupSessions immediately deletes all study sessions
func (a *AdminCommands) HandleCleanupSessions(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if user has admin permissions (you may want to add proper permission checking)
	if i.Member == nil || !hasAdminPermissions(i.Member) {
		respondWithError(s, i, "You don't have permission to use this command.")
		return
	}

	ctx := context.Background()

	// Count current sessions before deletion
	// Note: You'll need to run `sqlc generate` to get the new methods
	// For now, we'll use the existing DeleteOldStudySessions with a future date
	futureDate := time.Now().AddDate(1, 0, 0) // 1 year in the future

	err := a.db.DeleteOldStudySessions(ctx, futureDate)
	if err != nil {
		log.Printf("Error deleting all study sessions: %v", err)
		respondWithError(s, i, "Failed to delete study sessions.")
		return
	}

	response := "✅ All study sessions have been deleted. User statistics remain intact."

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: response,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("Error responding to cleanup command: %v", err)
	}

	log.Println("Admin command: All study sessions deleted")
}

// hasAdminPermissions checks if the member has admin permissions
func hasAdminPermissions(member *discordgo.Member) bool {
	// Simplified permission check - returns true for now
	// TODO: Implement proper role-based permission checking
	// You can check for specific role IDs or permission bits here
	return true // For now, allow all users - implement proper checks as needed
}

// respondWithError sends an error response
func respondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
