-- +goose Up
-- SQL in this section is executed when the migration is applied.
CREATE TABLE user_streaks (
    user_id TEXT NOT NULL,
    guild_id TEXT NOT NULL,
    current_streak_count INTEGER NOT NULL DEFAULT 0,
    max_streak_count INTEGER NOT NULL DEFAULT 0,
    last_activity_date DATE, -- Stores the UTC date
    streak_extended_today BOOLEAN NOT NULL DEFAULT FALSE,
    warning_notified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, guild_id)
);

CREATE INDEX IF NOT EXISTS idx_user_streaks_last_activity_date ON user_streaks(last_activity_date);
CREATE INDEX IF NOT EXISTS idx_user_streaks_current_streak_count ON user_streaks(current_streak_count);
CREATE INDEX IF NOT EXISTS idx_user_streaks_streak_extended_today ON user_streaks(streak_extended_today);


-- +goose Down
-- SQL in this section is executed when the migration is rolled back.
DROP TABLE IF EXISTS user_streaks; 