-- Emergency schema fix for production database
-- This addresses the bind parameter errors by adding missing columns

-- Add missing columns to user_streaks table
ALTER TABLE user_streaks ADD COLUMN IF NOT EXISTS last_activity_date DATE;
ALTER TABLE user_streaks ADD COLUMN IF NOT EXISTS streak_evaluated_date DATE;
ALTER TABLE user_streaks ADD COLUMN IF NOT EXISTS daily_activity_minutes INTEGER DEFAULT 0;
ALTER TABLE user_streaks ADD COLUMN IF NOT EXISTS activity_start_time TIMESTAMPTZ;
ALTER TABLE user_streaks ADD COLUMN IF NOT EXISTS streak_incremented_today BOOLEAN NOT NULL DEFAULT FALSE;

-- Update any existing records to have proper defaults
UPDATE user_streaks SET 
    daily_activity_minutes = COALESCE(daily_activity_minutes, 0),
    streak_incremented_today = COALESCE(streak_incremented_today, FALSE)
WHERE daily_activity_minutes IS NULL OR streak_incremented_today IS NULL;

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_user_streaks_last_activity_date ON user_streaks(last_activity_date);
CREATE INDEX IF NOT EXISTS idx_user_streaks_streak_evaluated_date ON user_streaks(streak_evaluated_date);
CREATE INDEX IF NOT EXISTS idx_user_streaks_daily_activity_minutes ON user_streaks(daily_activity_minutes);
CREATE INDEX IF NOT EXISTS idx_user_streaks_streak_incremented_today ON user_streaks(streak_incremented_today);

-- Clear any cached prepared statements to prevent parameter binding issues
DEALLOCATE ALL; 