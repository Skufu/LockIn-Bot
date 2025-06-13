-- LockIn-Bot Study Session Cleanup Script
-- Run this to immediately delete all study sessions
-- User statistics will remain intact

-- Show current count before deletion
SELECT 
    COUNT(*) as total_sessions,
    MIN(start_time) as oldest_session,
    MAX(start_time) as newest_session
FROM study_sessions;

-- Delete all study sessions
DELETE FROM study_sessions;

-- Verify deletion
SELECT COUNT(*) as remaining_sessions FROM study_sessions;

-- Show that user stats are still intact
SELECT 
    COUNT(*) as total_users_with_stats,
    SUM(total_study_ms) as total_study_time_preserved
FROM user_stats 
WHERE total_study_ms > 0;

-- Optional: Show storage space information (PostgreSQL specific)
SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as size
FROM pg_tables 
WHERE tablename IN ('study_sessions', 'user_stats', 'user_streaks')
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC; 