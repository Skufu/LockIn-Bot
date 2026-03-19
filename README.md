# LockIn Bot

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.24.1-blue.svg)](https://golang.org/dl/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-blue.svg)](https://www.postgresql.org/)

A professional Discord bot designed for gamifying studying through automatic voice channel tracking, streak systems, and leaderboards. Built with Go and optimized for study communities seeking to track and motivate consistent study habits.

## Features

- **Voice Channel Tracking**: Automatically tracks study sessions when users join configured voice channels
- **Personal Statistics**: Comprehensive study time analytics with daily, weekly, and monthly breakdowns
- **Server Leaderboards**: Competitive leaderboards to motivate study groups
- **Smart Streak System**: Calendar day-based streaks with Asia/Manila timezone support
- **Automated Notifications**: Evening warnings and streak celebrations
- **Historical Data**: Long-term study session tracking and analytics
- **Timezone Awareness**: Built-in Asia/Manila timezone handling for consistent streak calculations
- **Health Monitoring**: Built-in Discord token monitoring and alerts

## Quick Start

### Prerequisites

- **Go 1.24.1 or later**
- **PostgreSQL 17 or later**
- **Discord Bot Token** ([Create one here](https://discord.com/developers/applications))

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/Skufu/LockIn-Bot.git
   cd LockIn-Bot
   ```

2. **Configure environment variables**
   ```bash
   cp .env.example .env
   # Edit .env with your configuration
   ```

3. **Set up the database**
   ```bash
   # Install goose for migrations (if not already installed)
   go install github.com/pressly/goose/v3/cmd/goose@latest

   # Run migrations
   goose -dir db/migrations postgres "your_database_url" up
   ```

4. **Build and run**
   ```bash
   go mod download
   go build -o lockin-bot ./main.go
   ./lockin-bot
   ```

### Docker Deployment

```bash
docker build -t lockin-bot .
docker run --env-file .env lockin-bot
```

## Configuration

### Environment Variables

Create a `.env` file with the following variables:

```bash
# Discord Configuration
DISCORD_TOKEN=your_discord_bot_token
LOGGING_CHANNEL_ID=your_logging_channel_id
STREAK_NOTIFICATION_CHANNEL_ID=your_streak_channel_id

# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=lockinbot

# Voice Channel Tracking
ALLOWED_VOICE_CHANNEL_IDS=channel1,channel2,channel3
```

### Discord Bot Setup

1. Navigate to the [Discord Developer Portal](https://discord.com/developers/applications)
2. Create a new application and bot
3. Copy the bot token to your `.env` file
4. Invite the bot to your server with the following permissions:
   - Send Messages
   - Use Slash Commands
   - Read Message History
   - Connect to Voice Channels
   - View Channels

### Voice Channel Configuration

Add the Discord channel IDs of voice channels you want to track to `ALLOWED_VOICE_CHANNEL_IDS` as a comma-separated list. Users joining these channels will have their study time automatically tracked.

## Commands

| Command | Description |
|---------|-------------|
| `/stats` | Display your personal study statistics and rankings |
| `/leaderboard` | Show the server-wide study time leaderboard |
| `/streak` | Check your current study streak and progress |
| `/help` | Display available commands and bot information |

## Architecture


- **External Layer**: Discord command handlers and event processors
- **Application Layer**: Use case orchestration and input validation
- **Business Layer**: Core business logic and domain rules (streak calculations, statistics)
- **Data Layer**: PostgreSQL database operations with SQLC for type-safe queries

### Technology Stack

- **Language**: Go 1.24.1
- **Database**: PostgreSQL 17 (compatible with Neon, Render, Railway, etc.) with SQLC for type-safe queries
- **Discord API**: discordgo library
- **Deployment**: Docker (supports multiple platforms: Render, Railway, AWS, GCP, Azure)
- **Timezone Handling**: Asia/Manila timezone for consistent streak calculations
- **Architecture**: Clean Architecture with dependency injection

## Scheduled Operations

The bot runs several automated tasks:

- **11:59 PM Manila**: Daily streak evaluation and flag reset processing
- **8:00 PM Manila**: Evening activity warnings for users at risk of losing streaks
- **Midnight UTC**: Statistics resets (daily/weekly/monthly)
- **3:05 AM UTC**: Data pruning (removes old session records)

### Streak System Details

- **Minimum Activity**: 1 minute of voice channel activity per day
- **Calendar Day Basis**: Streaks are calculated based on Manila timezone calendar days
- **Immediate Feedback**: Users receive instant notifications when completing daily activity
- **Double-increment Protection**: Built-in safeguards prevent streak counting errors
- **Automatic Evaluation**: End-of-day processing ensures accurate streak maintenance

## Contributing

Contributions are welcome and encouraged. This is an open-source project and we appreciate community improvements.

### Getting Started

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes following the existing code style
4. Add tests for new functionality
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to your branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

### Development Guidelines

- Follow Go best practices and the existing Clean Architecture patterns
- Add tests for new features
- Update documentation as needed
- Ensure all timezone-related code uses Manila timezone helpers

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

If you encounter any issues or have questions:

1. Check the [Issues](https://github.com/Skufu/LockIn-Bot/issues) page
2. Create a new issue if your problem isn't already reported
3. Provide as much detail as possible including logs and configuration

## Discord Token Management

### Token Expiration

Discord bot tokens can expire or become invalid when:
- Token is manually regenerated in Discord Developer Portal
- Discord detects security issues and force-regenerates
- Token is manually revoked

### Automatic Detection & Alerts

LockIn Bot includes built-in token monitoring that:
- Checks Discord connection health every 30 seconds
- Detects token expiration automatically
- Sends alerts to your logging channel
- Provides clear instructions in logs
- Gracefully shuts down for restart

### Token Renewal Process

When token expiration is detected, follow these steps:

1. Navigate to [Discord Developer Portal](https://discord.com/developers/applications)
2. Select your bot application
3. Navigate to the 'Bot' section
4. Click 'Reset Token' to generate a new token
5. Copy the new token
6. Update your environment variables:
   ```bash
   # In your .env file or deployment environment
   DISCORD_TOKEN=your_new_token_here
   ```
7. Restart the bot service:
   ```bash
   # If using Docker/Render
   # The service will restart automatically due to exit code

   # If running locally
   go run main.go
   ```

### Security Best Practices

- Store tokens in environment variables, never in code
- Use `.env` files for local development
- Keep tokens secure in production environments
- Never commit tokens to version control
- Avoid sharing tokens in messages or public channels
