-- +goose Up
-- Repair user_streaks table to ensure all required columns exist

-- Add missing columns if they don't exist
ALTER TABLE user_streaks ADD COLUMN IF NOT EXISTS last_activity_date DATE;
ALTER TABLE user_streaks ADD COLUMN IF NOT EXISTS streak_evaluated_date DATE;
ALTER TABLE user_streaks ADD COLUMN IF NOT EXISTS daily_activity_minutes INTEGER DEFAULT 0;
ALTER TABLE user_streaks ADD COLUMN IF NOT EXISTS activity_start_time TIMESTAMPTZ;
ALTER TABLE user_streaks ADD COLUMN IF NOT EXISTS streak_incremented_today BOOLEAN NOT NULL DEFAULT FALSE;

-- Initialize default values for existing rows
UPDATE user_streaks SET
    daily_activity_minutes = 0
WHERE daily_activity_minutes IS NULL;

UPDATE user_streaks SET
    streak_incremented_today = FALSE
WHERE streak_incremented_today IS NULL;

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_user_streaks_last_activity_date ON user_streaks(last_activity_date);
CREATE INDEX IF NOT EXISTS idx_user_streaks_streak_evaluated_date ON user_streaks(streak_evaluated_date);
CREATE INDEX IF NOT EXISTS idx_user_streaks_daily_activity_minutes ON user_streaks(daily_activity_minutes);
CREATE INDEX IF NOT EXISTS idx_user_streaks_streak_incremented_today ON user_streaks(streak_incremented_today);

-- +goose Down
-- Remove the columns added in this migration
ALTER TABLE user_streaks DROP COLUMN IF EXISTS last_activity_date;
ALTER TABLE user_streaks DROP COLUMN IF EXISTS streak_evaluated_date;
ALTER TABLE user_streaks DROP COLUMN IF EXISTS daily_activity_minutes;
ALTER TABLE user_streaks DROP COLUMN IF EXISTS activity_start_time;
ALTER TABLE user_streaks DROP COLUMN IF EXISTS streak_incremented_today;

-- Drop indexes
DROP INDEX IF EXISTS idx_user_streaks_last_activity_date;
DROP INDEX IF EXISTS idx_user_streaks_streak_evaluated_date;
DROP INDEX IF EXISTS idx_user_streaks_daily_activity_minutes;
DROP INDEX IF EXISTS idx_user_streaks_streak_incremented_today; 