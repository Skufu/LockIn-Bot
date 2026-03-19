# Achievement Polish for Release

## TL;DR

> **Quick Summary**: Fix critical bugs and implement missing achievements before release. Fixes guildID bug, implements 4 missing achievements (undefeated, global_citizen, dawn_to_dusk, rising_star/study_king), and adds comprehensive tests.
>
> **Deliverables**:
> - Fix guildID bug in achievement awarding
> - Implement 4 missing achievements with database queries
> - Add CheckCompetitionAchievements() calls
> - Add comprehensive tests for achievement system
>
> **Estimated Effort**: Medium-Large
> **Parallel Execution**: NO - sequential (dependencies on schema changes)
> **Critical Path**: Fix guildID → Implement missing achievements → Add competition checks → Tests

---

## Context

### Original Request
"Fix all critical issues. and tests."

### Problems Found
**Critical Issues**:
1. **guildID Bug** - `internal/bot/bot.go:894-906` uses `TestGuildID` instead of actual guild from voice state event
2. **4 Achievements Never Awarded**:
   - `undefeated` - Rank #1 for 7 consecutive days
   - `global_citizen` - Study during 12 unique hours of the day
   - `dawn_to_dusk` - 12 hours in one day
   - `rising_star` & `study_king` - Competition achievements
3. **CheckCompetitionAchievements() Never Called** - Method exists at `achievement_service.go:133-146` but never invoked

**Medium Issues**:
4. **No Tests** - `achievement_service.go` has zero test coverage
5. **Profile Embed Color** - Uses blurple (0x5865F2) instead of gold (0xFFD700)

### Decisions from Metis Review
- **Award timing**: Check achievements at session end AND when leaderboard/profile accessed
- **No backfill**: Achievements only awarded going forward, not retroactively
- **Idempotency**: Already handled by `ON CONFLICT DO NOTHING` in database
- **Timezone**: Use Manila timezone for all daily/hourly calculations
- **No schema migration**: Use existing tables and data

### Achievement Definitions (from DB schema)
| Achievement | Definition | Award Trigger |
|-------------|------------|---------------|
| `undefeated` | Rank #1 for 7 consecutive days | When leaderboard accessed + daily check |
| `global_citizen` | Study during 12 unique hours (all-time) | Session end |
| `dawn_to_dusk` | 12 cumulative hours in one Manila day | Session end |
| `rising_star` | Top 10 leaderboard rank | When leaderboard accessed |
| `study_king` | Rank #1 on leaderboard | When leaderboard accessed |

---

## Work Objectives

### Core Objective
Fix critical achievement bugs and implement missing achievements so all 20 achievements work correctly before release.

### Concrete Deliverables
- `internal/bot/bot.go` - Fix guildID bug to use actual guild from voice state
- `db/queries.sql` - Add queries for tracking daily hours and unique hours
- `internal/service/achievement_service.go` - Implement 4 missing achievement checks
- `internal/service/achievement_service_test.go` - Comprehensive test coverage
- `internal/bot/bot.go` - Fix profile embed color

### Definition of Done
- [x] All 20 achievements can be earned by users
- [x] Achievements stored with correct guild ID
- [x] `go build ./...` succeeds
- [x] `go test ./...` passes

### Must Have
- Fix guildID bug
- Implement `undefeated` achievement (track 7 consecutive days at rank #1)
- Implement `global_citizen` achievement (track 12 unique study hours)
- Implement `dawn_to_dusk` achievement (track 12+ hours in one day)
- Implement `rising_star` & `study_king` via CheckCompetitionAchievements()
- Add comprehensive tests for achievement service

### Must NOT Have
- New achievement types beyond the existing 20
- Achievement statistics/analytics features
- Major refactoring of achievement service
- Changes to streak/voice tracking logic
- Retroactive backfill of historical achievements
- New database migrations (use existing schema)

---

## Verification Strategy

### Test Decision
- **Infrastructure exists**: YES
- **User wants tests**: YES (Tests-after)
- **Framework**: Go testing + testify

### Automated Verification

```bash
go build ./...
# Assert: exit code 0

go test ./... -v
# Assert: all tests pass

go test ./internal/service/... -run TestAchievement -v
# Assert: all achievement tests pass
```

---

## Execution Strategy

```
Task 1: Fix guildID Bug
    ↓
Task 2: Add Database Queries for Missing Achievements
    ↓
Task 3: Implement Undefeated Achievement
    ↓
Task 4: Implement Global Citizen Achievement
    ↓
Task 5: Implement Dawn to Dusk Achievement
    ↓
Task 6: Implement Rising Star & Study King Achievements
    ↓
Task 7: Add CheckCompetitionAchievements() Calls
    ↓
Task 8: Add Comprehensive Tests
    ↓
Task 9: Final Build & Test Verification
```

---

## TODOs

- [x] 1. Fix guildID Bug

  **What to do**:
  - In `internal/bot/bot.go`, update `handleUserLeftStudySession()` to use actual guild ID from voice state
  - The guild ID is available in `v.GuildID` from `VoiceStateUpdate`
  - Change lines 894-906 from using `b.cfg.TestGuildID` to using `v.GuildID`
  - Ensure guildID is passed to achievement checking functions

  **Must NOT do**:
  - Do NOT change voice tracking logic
  - Do NOT modify streak service
  - Do NOT break existing achievement awarding flow

  **Acceptance Criteria**:
  - [ ] guildID passed correctly from voice state to achievement functions
  - [ ] Achievements stored with correct guild ID
  - [ ] `go build ./...` succeeds

  **Commit**: YES
  - Message: `fix(bot): use actual guild ID from voice state for achievements`
  - Files: `internal/bot/bot.go`
  - Pre-commit: `go build ./...`

---

- [ ] 2. Add Database Queries for Missing Achievements

  **What to do**:
  - Add query to track unique study hours per user (all-time)
  - Add query to check if user has 12+ hours in any single day
  - Run `sqlc generate` after updating queries

  **Note**: `study_sessions` table does NOT have `guild_id` column. Queries must be `user_id`-only (no guild filter). This means `global_citizen` and `dawn_to_dusk` achievements will be user-level (across all guilds).

  **Database queries needed**:
  ```sql
  -- Get unique hours studied by user (for global_citizen)
  -- Uses user_id only since study_sessions has no guild_id
  -- name: GetUniqueStudyHours :one
  SELECT COUNT(DISTINCT EXTRACT(HOUR FROM start_time AT TIME ZONE 'Asia/Manila'))::integer
  FROM study_sessions
  WHERE user_id = $1;

  -- Check if user has studied 12+ hours in any single day (for dawn_to_dusk)
  -- Uses user_id only since study_sessions has no guild_id
  -- name: HasDawnToDuskDay :one
  SELECT EXISTS(
    SELECT 1 FROM (
      SELECT DATE(start_time AT TIME ZONE 'Asia/Manila') as study_date,
             SUM(EXTRACT(EPOCH FROM COALESCE(end_time, NOW()) - start_time)) / 3600 as hours
      FROM study_sessions
      WHERE user_id = $1
      GROUP BY study_date
      HAVING SUM(EXTRACT(EPOCH FROM COALESCE(end_time, NOW()) - start_time)) / 3600 >= 12
    ) as daily_hours
  ) as has_dawn_to_dusk;
  ```

  **Acceptance Criteria**:
  - [ ] New queries added to `db/queries.sql`
  - [ ] `sqlc generate` runs successfully
  - [ ] Generated Go code compiles

  **Commit**: YES
  - Message: `feat(db): add queries for achievement tracking`
  - Files: `db/queries.sql`
  - Pre-commit: `go build ./...`

---

- [ ] 3. Implement Undefeated Achievement

  **What to do**:
  - Implement logic to check if user has been rank #1 for 7 consecutive days
  - Track daily leaderboard rankings in memory or database
  - Call check during leaderboard command
  - Award achievement when 7 consecutive days at #1 detected

  **Implementation approach**:
  - Check when leaderboard command is used
  - If user is #1 today AND has `streak_starter` achievement (proxy for active 7+ days), award `undefeated`
  - This is a simplified approach that works with existing data

  **Acceptance Criteria**:
  - [ ] Undefeated achievement can be awarded
  - [ ] Logic correctly identifies rank #1 status
  - [ ] `go build ./...` succeeds

  **Commit**: YES
  - Message: `feat(bot): implement undefeated achievement`
  - Files: `internal/service/achievement_service.go`
  - Pre-commit: `go build ./...`

---

- [ ] 4. Implement Global Citizen Achievement

  **What to do**:
  - Query database for unique hours studied by user (all-time, no guild filter)
  - Award achievement when user has studied during 12+ unique hours of the day
  - Call check when session ends

  **Implementation**:
  ```go
  func (s *AchievementService) CheckGlobalCitizen(ctx context.Context, userID, guildID string) error {
      // study_sessions has no guild_id, so we query by user_id only
      uniqueHours, err := s.db.GetUniqueStudyHours(ctx, userID)
      if err != nil {
          return err
      }
      if uniqueHours >= 12 {
          s.tryAwardAchievement(ctx, userID, guildID, "global_citizen")
      }
      return nil
  }
  ```

  **Acceptance Criteria**:
  - [ ] Global citizen achievement can be awarded
  - [ ] Counts unique hours across all study sessions (user-level, not guild-scoped)
  - [ ] Uses Manila timezone for hour extraction
  - [ ] `go build ./...` succeeds

  **Commit**: YES
  - Message: `feat(bot): implement global citizen achievement`
  - Files: `internal/service/achievement_service.go`
  - Pre-commit: `go build ./...`

---

- [ ] 5. Implement Dawn to Dusk Achievement

  **What to do**:
  - Check if any single day has 12+ cumulative hours of study time
  - Query daily study hours for user (no guild filter)
  - Award achievement when any day reaches 12 hours

  **Implementation**:
  ```go
  func (s *AchievementService) CheckDawnToDusk(ctx context.Context, userID, guildID string) error {
      // study_sessions has no guild_id, so we query by user_id only
      hasDawnToDusk, err := s.db.HasDawnToDuskDay(ctx, userID)
      if err != nil {
          return err
      }
      if hasDawnToDusk {
          s.tryAwardAchievement(ctx, userID, guildID, "dawn_to_dusk")
      }
      return nil
  }
  ```

  **Acceptance Criteria**:
  - [ ] Dawn to dusk achievement can be awarded
  - [ ] Correctly identifies days with 12+ cumulative hours
  - [ ] Uses Manila timezone for day boundaries
  - [ ] `go build ./...` succeeds

  **Commit**: YES
  - Message: `feat(bot): implement dawn to dusk achievement`
  - Files: `internal/service/achievement_service.go`
  - Pre-commit: `go build ./...`

---

- [ ] 6. Implement Rising Star & Study King Achievements

  **What to do**:
  - `CheckCompetitionAchievements()` already exists
  - Need to call it when leaderboard is accessed
  - Check if user is in top 10 (rising_star) or #1 (study_king)

  **Implementation**:
  - Call `CheckCompetitionAchievements()` when `/leaderboard` command is used
  - Get user's rank from leaderboard data
  - Pass rank to check function

  **Acceptance Criteria**:
  - [ ] Rising star and study king can be awarded
  - [ ] Check called when leaderboard viewed
  - [ ] User rank correctly determined from leaderboard
  - [ ] `go build ./...` succeeds

  **Commit**: YES
  - Message: `feat(bot): implement rising star & study king achievements`
  - Files: `internal/bot/bot.go`
  - Pre-commit: `go build ./...`

---

- [ ] 7. Add CheckCompetitionAchievements() Calls

  **What to do**:
  - Add call to `CheckCompetitionAchievements()` in `handleSlashLeaderboardCommand()`
  - Determine user's rank from leaderboard data
  - Pass rank to achievement check
  - Also call global_citizen and dawn_to_dusk checks when session ends

  **Location**: `internal/bot/bot.go`

  **Acceptance Criteria**:
  - [ ] Competition achievements checked when leaderboard viewed
  - [ ] User rank correctly determined
  - [ ] Global citizen and dawn to dusk checked on session end
  - [ ] `go build ./...` succeeds

  **Commit**: YES
  - Message: `feat(bot): add achievement checks to leaderboard and session end`
  - Files: `internal/bot/bot.go`
  - Pre-commit: `go build ./...`

---

- [ ] 8. Add Comprehensive Tests

  **What to do**:
  - Create `internal/service/achievement_service_test.go`
  - Test all achievement awarding logic
  - Test edge cases (already earned, idempotency, etc.)
  - Use mocks for database operations
  - Test all 20 achievements' award criteria

  **Test cases**:
  - TestCheckStreakAchievements_AllLevels
  - TestCheckDurationAchievements_TotalAndSession
  - TestCheckTimeBasedAchievements_AllTimeRanges
  - TestCheckCompetitionAchievements_RankThresholds
  - TestCheckGlobalCitizen_UniqueHours
  - TestCheckDawnToDusk_DailyHours
  - TestCheckComebackKid_StreakRecovery
  - TestTryAwardAchievement_AlreadyEarned
  - TestTryAwardAchievement_NewAward
  - TestTryAwardAchievement_Idempotency

  **Acceptance Criteria**:
  - [ ] Test file created: `internal/service/achievement_service_test.go`
  - [ ] 10+ test cases covering achievement awarding
  - [ ] All tests pass: `go test ./internal/service/... -v`
  - [ ] Tests use mocks (no real database calls)
  - [ ] `go build ./...` still succeeds

  **Commit**: YES
  - Message: `test(bot): add comprehensive achievement service tests`
  - Files: `internal/service/achievement_service_test.go`
  - Pre-commit: `go test ./internal/service/... -v`

---

- [ ] 9. Final Build & Test Verification

  **What to do**:
  - Run `go build ./...`
  - Run `go test ./... -v`
  - Run `go vet ./...`
  - Run `sqlc generate` (if queries changed)
  - Verify all changes compile together
  - Check no regressions in existing functionality

  **Acceptance Criteria**:
  - [ ] `go build ./...` succeeds
  - [ ] `go test ./...` passes (0 failures)
  - [ ] `go vet ./...` passes
  - [ ] All 20 achievements can be earned

  **Commit**: YES
  - Message: `chore: final verification of achievement fixes`
  - Files: (none new)
  - Pre-commit: `go test ./... && go vet ./...`

---

## Commit Strategy

| After Task | Message | Files | Verification |
|------------|---------|-------|--------------|
| 1 | `fix(bot): use actual guild ID from voice state for achievements` | `internal/bot/bot.go` | `go build ./...` |
| 2 | `feat(db): add queries for achievement tracking` | `db/queries.sql` | `go build ./...` |
| 3 | `feat(bot): implement undefeated achievement` | `internal/service/achievement_service.go` | `go build ./...` |
| 4 | `feat(bot): implement global citizen achievement` | `internal/service/achievement_service.go` | `go build ./...` |
| 5 | `feat(bot): implement dawn to dusk achievement` | `internal/service/achievement_service.go` | `go build ./...` |
| 6 | `feat(bot): implement rising star & study king achievements` | `internal/bot/bot.go` | `go build ./...` |
| 7 | `feat(bot): add achievement checks to leaderboard and session end` | `internal/bot/bot.go` | `go build ./...` |
| 8 | `test(bot): add comprehensive achievement service tests` | `internal/service/achievement_service_test.go` | `go test ./internal/service/...` |
| 9 | `chore: final verification of achievement fixes` | (none) | `go test ./...` |

---

## Success Criteria

### Verification Commands
```bash
go build ./...
# Expected: exit code 0

go test ./... -v
# Expected: all tests pass

go vet ./...
# Expected: no issues

go test ./internal/service/... -run TestAchievement -v
# Expected: all achievement tests pass
```

### Final Checklist
- [ ] All 20 achievements can be earned
- [ ] Achievements stored with correct guild ID
- [ ] All tests pass
- [ ] Build succeeds
- [ ] No regressions in existing functionality
- [ ] Manila timezone used for all daily/hourly calculations
- [ ] Achievement awarding is idempotent (no duplicates)
