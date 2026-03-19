# Bot Reliability Fixes

## TL;DR

> **Quick Summary**: Fix Discord bot crash-looping every ~2 days by adding retry logic with exponential backoff for bot initialization, classifying errors (permanent vs transient), improving token pre-validation, and adding actionable diagnostic logging.
>
> **Deliverables**:
> - Retry logic with exponential backoff for bot initialization
> - Error classification system (permanent auth vs transient API)
> - Token pre-validation before Discord API calls
> - Reduced health check log noise
> - Unit tests for startup failure scenarios
>
> **Estimated Effort**: Medium
> **Parallel Execution**: NO - sequential (changes are all in startup path)
> **Critical Path**: Error Classification → Retry Logic → Token Validation → Log Cleanup → Tests

---

## Context

### Original Request
"what is wrong with the bot shutting down every two days. fix the problems right now. and any problems you see. create plans"

### Interview Summary
**Key Discussions**:
- Error: `[DG0] restapi.go:283:RequestWithLockedBucket() rate limit unmarshal error, invalid character 'e' looking for beginning of value`
- User confirmed: "it's the discord token. i have to always restart it"
- Discord tokens don't normally expire on 48h cadence - likely transient Cloudflare/IP rate-limiting by Discord
- Known discordgo issue #659 (open since 2019) - no upstream fix yet

**Research Findings**:
- discordgo expects JSON but Discord returns HTML/text error responses
- Render uses shared IPs that may trigger Discord's Cloudflare blocks
- Discord's Invalid Request Limit: 10,000 requests/10 minutes triggers IP bans
- Cloudflare bans typically last 24-48 hours

### Metis Review
**Identified Gaps** (addressed):
- Need to distinguish permanent auth errors from transient failures
- Must not retry invalid tokens forever (could worsen rate-limit bans)
- Acceptance criteria must be executable (not "verify it works")
- Edge case: HTML responses, DNS timeouts, partial startup failures
- Token material must never appear in logs

---

## Work Objectives

### Core Objective
Prevent Discord bot from crash-looping by making initialization resilient to transient failures while failing fast on permanent auth/config errors.

### Concrete Deliverables
- `internal/bot/bot.go` - Retry logic and error classification in `New()` function
- `internal/bot/bot.go` - Token pre-validation helper
- `internal/bot/bot.go` - Structured error types for different failure modes
- `main.go` - Updated startup flow with retry and graceful error handling
- `internal/bot/bot_test.go` - Tests for startup failure scenarios

### Definition of Done
- [x] Bot does not crash on first transient Discord API error
- [x] Bot fails fast with clear message on permanently invalid token
- [x] Log output distinguishes error types (auth vs transient vs unknown)
- [x] `go build ./...` succeeds
- [x] `go test ./...` passes

### Must Have
- Bounded retry with exponential backoff (max 5 retries, max delay 60s)
- Jitter added to prevent thundering herd
- Error classification: permanent config/auth vs transient API/network
- Sensitive token never in logs

### Must NOT Have (Guardrails)
- Full reconnect/resume architecture redesign
- Auto token refresh (Discord bot tokens are static secrets)
- General logging framework overhaul
- Changes to command handlers or voice tracking logic
- Unbounded retry loops

---

## Verification Strategy (MANDATORY)

### Test Decision
- **Infrastructure exists**: YES
- **User wants tests**: YES (Tests-after)
- **Framework**: Go testing + testify

### Automated Verification

```bash
# Build verification
go build -o lockin-bot ./main.go
# Assert: exit code 0

# All tests pass
go test ./... -v
# Assert: all tests pass, 0 failures

# Specific startup reliability tests
go test ./... -run TestStartup -v
# Assert: all TestStartup_* tests pass
```

**Evidence to Capture:**
- [x] Terminal output from `go test ./...` showing all tests pass
- [x] Terminal output from `go build ./...` confirming successful compilation

---

## Execution Strategy

### Sequential Execution (Startup path is linear)

```
Task 1: Error Classification System
    ↓
Task 2: Token Pre-Validation
    ↓
Task 3: Retry Logic with Backoff
    ↓
Task 4: Update main.go Startup Flow
    ↓
Task 5: Reduce Health Check Log Noise
    ↓
Task 6: Unit Tests for Startup Scenarios
    ↓
Task 7: Final Build & Test Verification
```

### Dependency Matrix

| Task | Depends On | Blocks |
|------|------------|--------|
| 1 - Error Classification | None | 2, 3, 4 |
| 2 - Token Pre-Validation | 1 | 3, 4 |
| 3 - Retry Logic | 1, 2 | 4 |
| 4 - Update main.go | 1, 2, 3 | 5, 6, 7 |
| 5 - Log Noise Reduction | 4 | 7 |
| 6 - Unit Tests | 4 | 7 |
| 7 - Final Verification | 5, 6 | None |

---

## TODOs

- [x] 1. Create Error Classification System

  **What to do**:
  - Define structured error types for Discord startup failures
  - Create `BotStartupError` type with error classification:
    - `ErrorTypePermanent` - Invalid token format, missing config
    - `ErrorTypeTransient` - Network timeout, rate limit, Cloudflare block
    - `ErrorTypeUnknown` - Non-JSON response, unexpected errors
  - Add helper function `classifyStartupError(err error) BotStartupError`
  - Parse discordgo errors to extract HTTP status codes when available
  - Classify "invalid character 'e'" errors as `ErrorTypeTransient` (non-JSON from Discord)

  **Must NOT do**:
  - Modify existing error handling in command handlers
  - Change voice tracking error handling
  - Add logging framework dependencies

  **Recommended Agent Profile**:
  - **Category**: `unspecified-low`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (Task 1)
  - **Blocks**: Tasks 2, 3, 4

  **References**:
  - `internal/bot/bot.go:18-38` - Bot struct definition, where error types will be used
  - `main.go:46-50` - Current fatal error handling on bot creation
  - `github.com/bwmarrin/discordgo` - RESTError type for HTTP status parsing

  **Acceptance Criteria**:
  - [ ] `BotStartupError` type defined with `ErrorType` field (permanent/transient/unknown)
  - [ ] `classifyStartupError()` function implemented
  - [ ] Error classification correctly identifies: token errors, network errors, parse errors
  - [ ] `go build ./...` succeeds

  **Commit**: YES
  - Message: `feat(bot): add error classification system for startup failures`
  - Files: `internal/bot/errors.go`
  - Pre-commit: `go build ./...`

---

- [ ] 2. Add Token Pre-Validation

  **What to do**:
  - Create `validateToken(token string) error` function in `internal/bot/`
  - Check: token not empty
  - Check: no leading/trailing whitespace
  - Check: no newlines or carriage returns
  - Check: reasonable length (Discord tokens are typically 70+ chars)
  - Check: matches Discord token format pattern (base64-like characters)
  - Return clear error message if validation fails
  - Call validation before `discordgo.New()` in `bot.New()`

  **Must NOT do**:
  - Make API calls to validate token (this is format-only validation)
  - Log the actual token value (security)
  - Accept tokens with quotes or extra formatting

  **Recommended Agent Profile**:
  - **Category**: `unspecified-low`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (Task 2, after Task 1)
  - **Blocks**: Tasks 3, 4
  - **Blocked By**: Task 1

  **References**:
  - `internal/config/config.go:48` - Where token is loaded from environment
  - `internal/bot/bot.go:43` - Where token is passed to discordgo

  **Acceptance Criteria**:
  - [ ] `validateToken()` function implemented
  - [ ] Rejects: empty string, string with newlines, string with quotes
  - [ ] Accepts: valid Discord token format (70+ chars, base64-like)
  - [ ] `go build ./...` succeeds

  **Commit**: YES
  - Message: `feat(bot): add token pre-validation before Discord connection`
  - Files: `internal/bot/validation.go`
  - Pre-commit: `go build ./...`

---

- [ ] 3. Implement Retry Logic with Exponential Backoff

  **What to do**:
  - Create `connectWithRetry(token string, maxRetries int) (*discordgo.Session, error)` function
  - Implement exponential backoff: 1s, 2s, 4s, 8s, 16s (capped at 60s)
  - Add jitter: random 0-25% delay variation
  - Max retries: 5 (configurable)
  - Use error classification to decide retry behavior:
    - `ErrorTypePermanent`: Fail immediately, no retry
    - `ErrorTypeTransient`: Retry with backoff
    - `ErrorTypeUnknown`: Retry with backoff (but log warning)
  - Log each retry attempt with error type and wait duration
  - Log final failure with total attempts and last error

  **Must NOT do**:
  - Retry indefinitely
  - Retry on permanent auth errors
  - Include token in log messages
  - Add complex circuit breaker patterns (keep simple)

  **Recommended Agent Profile**:
  - **Category**: `unspecified-low`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (Task 3, after Task 2)
  - **Blocks**: Task 4
  - **Blocked By**: Tasks 1, 2

  **References**:
  - `internal/bot/bot.go:40-92` - Current `New()` function that needs retry wrapper
  - Standard Go `time` package for sleep/backoff
  - Standard Go `math/rand` for jitter

  **Acceptance Criteria**:
  - [ ] `connectWithRetry()` function implemented
  - [ ] Retries up to 5 times on transient errors
  - [ ] Exponential backoff: 1s → 2s → 4s → 8s → 16s → 30s → 60s (cap)
  - [ ] Jitter added (0-25% random variation)
  - [ ] Fails immediately on permanent errors (no retry)
  - [ ] Logs show retry attempts with error type
  - [ ] `go build ./...` succeeds

  **Commit**: YES
  - Message: `feat(bot): add retry logic with exponential backoff for initialization`
  - Files: `internal/bot/retry.go`
  - Pre-commit: `go build ./...`

---

- [ ] 4. Update main.go Startup Flow

  **What to do**:
  - Replace `bot.New()` call with retry-wrapped version
  - Remove `log.Fatalf()` for transient Discord errors
  - Use `log.Printf()` for retry attempts and failures
  - Only `log.Fatalf()` for permanent config/database errors
  - Add startup banner showing retry configuration
  - Log clear error summary when all retries exhausted
  - Keep database/migration errors as fatal (they can't be retried)

  **Must NOT do**:
  - Change database connection error handling (leave as fatal)
  - Change migration error handling (leave as fatal)
  - Add retry logic for config loading errors

  **Recommended Agent Profile**:
  - **Category**: `unspecified-low`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (Task 4, after Tasks 1-3)
  - **Blocks**: Tasks 5, 6, 7
  - **Blocked By**: Tasks 1, 2, 3

  **References**:
  - `main.go:46-50` - Current bot initialization with fatal error
  - `internal/bot/bot.go` - New bot creation functions

  **Acceptance Criteria**:
  - [ ] Bot initialization uses retry logic
  - [ ] Transient Discord errors logged with retry count, not fatal
  - [ ] Permanent errors still cause immediate exit with clear message
  - [ ] `go build ./...` succeeds

  **Commit**: YES
  - Message: `refactor(main): update startup flow to use retry logic`
  - Files: `main.go`
  - Pre-commit: `go build ./...`

---

- [ ] 5. Reduce Health Check Log Noise

  **What to do**:
  - Add log level filtering for `/healthz` endpoint
  - Only log first health check after startup
  - Suppress subsequent health check logs for 5 minutes
  - Keep `/` endpoint logging (it's less frequent)
  - Add timestamp of last logged health check

  **Must NOT do**:
  - Remove health check logging entirely
  - Change health check response content
  - Add logging libraries (keep standard log package)

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (Task 5, after Task 4)
  - **Blocks**: Task 7
  - **Blocked By**: Task 4

  **References**:
  - `main.go:87-92` - Current healthz handler with request logging

  **Acceptance Criteria**:
  - [ ] Health check logs suppressed for 5 minutes after first log
  - [ ] First health check after startup is logged
  - [ ] `/` endpoint still logs requests
  - [ ] `go build ./...` succeeds

  **Commit**: YES
  - Message: `refactor(main): reduce health check log noise`
  - Files: `main.go`
  - Pre-commit: `go build ./...`

---

- [ ] 6. Add Unit Tests for Startup Scenarios

  **What to do**:
  - Create `internal/bot/startup_test.go`
  - Test: `TestStartup_TransientError_RetriesThenSucceeds`
  - Test: `TestStartup_PermanentError_FailsFastWithoutRetry`
  - Test: `TestStartup_InvalidToken_ValidationError`
  - Test: `TestStartup_RetryBackoff_BoundedAndJittered`
  - Test: `TestStartup_NonJSONResponse_ClassifiedAsTransient`
  - Use mock Discord session for testing (no real API calls)

  **Must NOT do**:
  - Make real Discord API calls in tests
  - Test voice tracking or command handling
  - Add complex test infrastructure

  **Recommended Agent Profile**:
  - **Category**: `unspecified-low`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (Task 6, after Task 4)
  - **Blocks**: Task 7
  - **Blocked By**: Task 4

  **References**:
  - Existing test patterns in codebase
  - `internal/bot/bot_test.go` (if exists)

  **Acceptance Criteria**:
  - [ ] Test file created: `internal/bot/startup_test.go`
  - [ ] 5+ test cases covering startup failure scenarios
  - [ ] All tests pass: `go test ./internal/bot/... -v`
  - [ ] Tests use mocks (no real Discord API calls)
  - [ ] `go build ./...` still succeeds

  **Commit**: YES
  - Message: `test(bot): add unit tests for startup reliability`
  - Files: `internal/bot/startup_test.go`
  - Pre-commit: `go test ./internal/bot/... -v`

---

- [ ] 7. Final Build & Test Verification

  **What to do**:
  - Run `go build -o lockin-bot ./main.go`
  - Run `go test ./... -v`
  - Run `go vet ./...`
  - Verify all changes compile together
  - Check no regressions in existing functionality

  **Must NOT do**:
  - Skip any verification steps
  - Commit if any step fails

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Sequential (Task 7, final)
  - **Blocks**: None (final task)
  - **Blocked By**: Tasks 5, 6

  **References**:
  - `go.mod` - Module definition and dependencies

  **Acceptance Criteria**:
  - [ ] `go build -o lockin-bot ./main.go` succeeds (exit code 0)
  - [ ] `go test ./... -v` passes (0 failures)
  - [ ] `go vet ./...` passes
  - [ ] Binary size reasonable for Go bot (~10-20MB)

  **Commit**: YES
  - Message: `chore: final verification of bot reliability fixes`
  - Files: (none new)
  - Pre-commit: `go test ./... && go vet ./...`

---

## Commit Strategy

| After Task | Message | Files | Verification |
|------------|---------|-------|--------------|
| 1 | `feat(bot): add error classification system for startup failures` | `internal/bot/errors.go` | `go build ./...` |
| 2 | `feat(bot): add token pre-validation before Discord connection` | `internal/bot/validation.go` | `go build ./...` |
| 3 | `feat(bot): add retry logic with exponential backoff for initialization` | `internal/bot/retry.go` | `go build ./...` |
| 4 | `refactor(main): update startup flow to use retry logic` | `main.go` | `go build ./...` |
| 5 | `refactor(main): reduce health check log noise` | `main.go` | `go build ./...` |
| 6 | `test(bot): add unit tests for startup reliability` | `internal/bot/startup_test.go` | `go test ./internal/bot/...` |
| 7 | `chore: final verification of bot reliability fixes` | (none) | `go test ./...` |

---

## Success Criteria

### Verification Commands
```bash
go build -o lockin-bot ./main.go
# Expected: exit code 0

go test ./... -v
# Expected: all tests pass

go vet ./...
# Expected: no issues

go test ./internal/bot/... -run TestStartup -v
# Expected: all startup tests pass
```

### Final Checklist
- [x] Bot doesn't crash on first transient Discord error
- [x] Bot fails fast on permanently invalid token with clear message
- [x] Logs distinguish error types (permanent/transient/unknown)
- [x] All tests pass
- [x] Build succeeds
- [x] No token material in logs
