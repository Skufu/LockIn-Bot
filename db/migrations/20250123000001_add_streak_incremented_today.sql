-- +goose Up
-- Add field to track if today's streak was already incremented
ALTER TABLE user_streaks ADD COLUMN streak_incremented_today BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
-- Remove the field
ALTER TABLE user_streaks DROP COLUMN streak_incremented_today; 