# LockIn-Bot

A Discord bot for logging studying through voice channel tracking, streak systems, and leaderboards.

## Features

- 🎧 **Voice Channel Tracking:** Auto-detect study sessions when users join tracked voice channels
- 📊 **Statistics:** Personal stats & server leaderboards showing study time
- 🔥 **Streak System:** Calendar day-based streaks in Manila timezone
- ⏰ **Scheduled Tasks:** Automatic resets & notifications

## Tech Stack

- **Go:** 1.24.1 with Clean Architecture
- **Database:** PostgreSQL 17 + SQLC
- **Discord:** discordgo library
- **Deployment:** Docker + Render
- **Timezone:** Asia/Manila for all streak calculations

## Installation

1. Clone the repository
2. Set up environment variables (see `.env.example`)
3. Run database migrations
4. Build and run the bot

```bash
go build
./LockIn-Bot
```

## Environment Variables

```bash
DISCORD_TOKEN=your_discord_bot_token
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=lockinbot
LOGGING_CHANNEL_ID=your_logging_channel_id
STREAK_NOTIFICATION_CHANNEL_ID=your_streak_channel_id
ALLOWED_VOICE_CHANNEL_IDS=channel1,channel2,channel3
```

## Configuration

The bot requires specific voice channels to be configured for tracking. Add the channel IDs to the `ALLOWED_VOICE_CHANNEL_IDS` environment variable as a comma-separated list.

## Commands

- `/stats` - Shows your study/voice channel time statistics
- `/leaderboard` - Shows the study time leaderboard
- `/streak` - Check your current study streak
- `/help` - Shows available commands and information about the bot


## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some feature'`)
4. Push to the branch (`git push origin feature/feature`)
5. Open a Pull Request
