# LockIn-Bot Maintenance Scripts

This directory contains utility scripts for maintaining and cleaning up the LockIn-Bot database.

## 🧹 Study Session Cleanup

### Problem
Study sessions can accumulate quickly and consume database storage. Each voice channel join/leave creates a record, potentially generating thousands of entries per week.

### Solutions

#### Option 1: Go Script (Recommended)
```bash
# Run the cleanup script
go run scripts/cleanup_sessions.go
```

**Features:**
- Safely deletes ALL study sessions
- Preserves user statistics
- Uses existing database connection logic
- Provides detailed feedback

#### Option 2: Direct SQL
```bash
# Connect to your database and run:
psql your_database_url -f scripts/cleanup_sessions.sql
```

**Features:**
- Shows before/after statistics
- Provides storage size information
- Can be run from any SQL client

### ⚠️ Important Notes

1. **User Statistics Are Safe**: All study time totals remain in the `user_stats` table
2. **Streak Data Is Safe**: All streak information remains in the `user_streaks` table  
3. **Only Raw Sessions Deleted**: Individual session records are removed to save space
4. **Automatic Prevention**: The bot now deletes sessions older than 1 week automatically

### 🔄 Automatic Cleanup (Already Implemented)

The bot scheduler has been updated to:
- Delete study sessions older than **1 week** (instead of 6 months)
- Run daily at 3:05 AM server time
- Prevent storage buildup going forward

### 📊 What Gets Deleted vs Preserved

**✅ PRESERVED:**
- User total study time (`user_stats.total_study_ms`)
- Daily/weekly/monthly statistics (`user_stats`)
- Streak counts and history (`user_streaks`)
- User information (`users`)

**❌ DELETED:**
- Individual session start/end times (`study_sessions`)
- Session durations (already aggregated into stats)

### 🚀 Recommended Action Plan

1. **Immediate**: Run cleanup script to free storage now
2. **Future**: Let automatic weekly cleanup handle ongoing maintenance
3. **Database Migration**: Consider migrating to Railway PostgreSQL for better free limits

### Usage Examples

```bash
# Check current session count first
psql your_db_url -c "SELECT COUNT(*) FROM study_sessions;"

# Run cleanup
go run scripts/cleanup_sessions.go

# Verify cleanup worked
psql your_db_url -c "SELECT COUNT(*) FROM study_sessions;"
``` 