-- +goose Up
-- Adding new columns required by the revamped streak system.
-- Note: This migration is idempotent and safe to run multiple times.

-- 1. Add new columns with sensible defaults (will skip if columns already exist)
ALTER TABLE user_streaks
    ADD COLUMN IF NOT EXISTS daily_activity_minutes INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS activity_start_time TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS streak_incremented_today BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS streak_evaluated_date DATE;

-- 2. Legacy column migration (will be skipped since the column doesn't exist)
-- Note: Since the legacy column 'streak_extended_today' doesn't exist in the current schema,
-- we don't need to perform any data migration. The new columns are already properly initialized.

-- 3. Refresh indexes to align with the new column names
DROP INDEX IF EXISTS idx_user_streaks_streak_extended_today;

CREATE INDEX IF NOT EXISTS idx_user_streaks_last_activity_date ON user_streaks(last_activity_date);
CREATE INDEX IF NOT EXISTS idx_user_streaks_streak_incremented_today ON user_streaks(streak_incremented_today);

-- +goose Down
-- Revert the schema changes (best-effort).

-- 1. Restore the legacy flag column
ALTER TABLE user_streaks
    ADD COLUMN IF NOT EXISTS streak_extended_today BOOLEAN NOT NULL DEFAULT FALSE;

-- 2. Move data back into the legacy column
UPDATE user_streaks
SET streak_extended_today = COALESCE(streak_incremented_today, FALSE);

-- 3. Drop the new columns that did not previously exist
ALTER TABLE user_streaks
    DROP COLUMN IF EXISTS streak_evaluated_date,
    DROP COLUMN IF EXISTS streak_incremented_today,
    DROP COLUMN IF EXISTS activity_start_time,
    DROP COLUMN IF EXISTS daily_activity_minutes;

-- 4. Restore original indexes
DROP INDEX IF EXISTS idx_user_streaks_streak_incremented_today;
CREATE INDEX IF NOT EXISTS idx_user_streaks_streak_extended_today ON user_streaks(streak_extended_today); 