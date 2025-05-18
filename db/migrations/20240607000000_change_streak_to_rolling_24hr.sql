-- +goose Up
-- SQL in this section is executed when the migration is applied.

-- Step 1: Add the new timestamp column for streak activity
ALTER TABLE user_streaks ADD COLUMN last_streak_activity_timestamp TIMESTAMPTZ;

-- Step 2: Populate the new timestamp column from the old date column.
-- We assume the activity occurred at the beginning of the UTC date.
-- If last_activity_date was NULL, last_streak_activity_timestamp will also be NULL.
UPDATE user_streaks
SET last_streak_activity_timestamp = last_activity_date::TIMESTAMPTZ
WHERE last_activity_date IS NOT NULL;

-- Step 3: Drop the old last_activity_date column
ALTER TABLE user_streaks DROP COLUMN last_activity_date;

-- Step 4: Drop the streak_extended_today column as it's no longer needed
ALTER TABLE user_streaks DROP COLUMN streak_extended_today;

-- Step 5: Update indexes if necessary.
-- Drop the old index on last_activity_date if it exists by its specific name.
-- The exact name might vary based on how it was created (e.g., idx_user_streaks_last_activity_date).
-- You might need to look up the exact index name if this fails.
DROP INDEX IF EXISTS idx_user_streaks_last_activity_date;
DROP INDEX IF EXISTS idx_user_streaks_streak_extended_today; -- If an index existed for this

-- Create a new index on the new timestamp column
CREATE INDEX IF NOT EXISTS idx_user_streaks_last_streak_activity_timestamp ON user_streaks(last_streak_activity_timestamp);

-- +goose Down
-- SQL in this section is executed when the migration is rolled back.

-- Step 1: Add back streak_extended_today column
ALTER TABLE user_streaks ADD COLUMN streak_extended_today BOOLEAN NOT NULL DEFAULT FALSE;

-- Step 2: Add back last_activity_date column
ALTER TABLE user_streaks ADD COLUMN last_activity_date DATE;

-- Step 3: Populate last_activity_date from last_streak_activity_timestamp
-- This will truncate the time part, effectively restoring the old date logic.
UPDATE user_streaks
SET last_activity_date = last_streak_activity_timestamp::DATE
WHERE last_streak_activity_timestamp IS NOT NULL;

-- Step 4: Drop the last_streak_activity_timestamp column
ALTER TABLE user_streaks DROP COLUMN last_streak_activity_timestamp;

-- Step 5: Recreate old indexes.
DROP INDEX IF EXISTS idx_user_streaks_last_streak_activity_timestamp;
CREATE INDEX IF NOT EXISTS idx_user_streaks_last_activity_date ON user_streaks(last_activity_date);
CREATE INDEX IF NOT EXISTS idx_user_streaks_streak_extended_today ON user_streaks(streak_extended_today); 