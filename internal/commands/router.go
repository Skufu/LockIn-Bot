package commands

import (
	"context"
	"strings"

	"github.com/Skufu/LockIn-Bot/internal/database"
	"github.com/bwmarrin/discordgo"
)

// CommandHandler is a function that handles a specific command
type CommandHandler func(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, args []string)

// Router handles command routing and execution
type Router struct {
	db          *database.Queries
	prefix      string
	commands    map[string]CommandHandler
	description map[string]string
}

// NewRouter creates a new command router
func NewRouter(db *database.Queries, prefix string) *Router {
	return &Router{
		db:          db,
		prefix:      prefix,
		commands:    make(map[string]CommandHandler),
		description: make(map[string]string),
	}
}

// Register registers a command with the router
func (r *Router) Register(name string, description string, handler CommandHandler) {
	name = strings.ToLower(name)
	r.commands[name] = handler
	r.description[name] = description
}

// HandleMessage handles incoming messages and routes them to the appropriate command handler
func (r *Router) HandleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from bots
	if m.Author.Bot {
		return
	}

	// Check if message starts with prefix
	if !strings.HasPrefix(m.Content, r.prefix) {
		return
	}

	// Remove prefix and split into command and args
	content := strings.TrimPrefix(m.Content, r.prefix)
	parts := strings.Fields(content)
	if len(parts) == 0 {
		return
	}

	// Get command name and args
	cmdName := strings.ToLower(parts[0])
	args := parts[1:]

	// Find and execute command
	if handler, exists := r.commands[cmdName]; exists {
		// Create context with DB and router
		ctx := context.Background()
		ctx = context.WithValue(ctx, DBKey, r.db)
		ctx = context.WithValue(ctx, "router", r)

		handler(ctx, s, m, args)
	}
}

// GetHelpText returns help text for all registered commands
func (r *Router) GetHelpText() string {
	var helpText strings.Builder
	helpText.WriteString("**Available Commands**\n")

	for cmd, desc := range r.description {
		helpText.WriteString("• `")
		helpText.WriteString(r.prefix)
		helpText.WriteString(cmd)
		helpText.WriteString("` - ")
		helpText.WriteString(desc)
		helpText.WriteString("\n")
	}

	return helpText.String()
}
