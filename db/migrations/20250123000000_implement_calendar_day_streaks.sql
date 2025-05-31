-- +goose Up
-- SQL in this section is executed when the migration is applied.

-- Add new date-based fields for calendar day streak tracking
ALTER TABLE user_streaks ADD COLUMN last_activity_date DATE;
ALTER TABLE user_streaks ADD COLUMN streak_evaluated_date DATE;
ALTER TABLE user_streaks ADD COLUMN daily_activity_minutes INTEGER DEFAULT 0;
ALTER TABLE user_streaks ADD COLUMN activity_start_time TIMESTAMPTZ;

-- Reset all existing streaks to start fresh with the new system
UPDATE user_streaks SET 
    current_streak_count = 0,
    last_activity_date = NULL,
    streak_evaluated_date = NULL,
    daily_activity_minutes = 0,
    activity_start_time = NULL,
    warning_notified_at = NULL,
    updated_at = NOW();

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_user_streaks_last_activity_date ON user_streaks(last_activity_date);
CREATE INDEX IF NOT EXISTS idx_user_streaks_streak_evaluated_date ON user_streaks(streak_evaluated_date);
CREATE INDEX IF NOT EXISTS idx_user_streaks_daily_activity_minutes ON user_streaks(daily_activity_minutes);

-- +goose Down
-- SQL in this section is executed when the migration is rolled back.

-- Remove the new columns
ALTER TABLE user_streaks DROP COLUMN IF EXISTS last_activity_date;
ALTER TABLE user_streaks DROP COLUMN IF EXISTS streak_evaluated_date;
ALTER TABLE user_streaks DROP COLUMN IF EXISTS daily_activity_minutes;
ALTER TABLE user_streaks DROP COLUMN IF EXISTS activity_start_time;

-- Drop the indexes
DROP INDEX IF EXISTS idx_user_streaks_last_activity_date;
DROP INDEX IF EXISTS idx_user_streaks_streak_evaluated_date;
DROP INDEX IF EXISTS idx_user_streaks_daily_activity_minutes; 