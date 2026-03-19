package bot

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestClassifyStartupError_TokenError tests that token/auth errors are classified as permanent
func TestClassifyStartupError_TokenError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
	}{
		{
			name:         "401 unauthorized error is permanent",
			err:          errors.New("401 Unauthorized"),
			expectedType: ErrorTypePermanent,
		},
		{
			name:         "403 forbidden error is permanent",
			err:          errors.New("403 Forbidden: Missing Access"),
			expectedType: ErrorTypePermanent,
		},
		{
			name:         "invalid token error is permanent",
			err:          errors.New("invalid token provided"),
			expectedType: ErrorTypePermanent,
		},
		{
			name:         "authentication failed error is permanent",
			err:          errors.New("authentication failed"),
			expectedType: ErrorTypePermanent,
		},
		{
			name:         "token unauthorized error is permanent",
			err:          errors.New("token unauthorized"),
			expectedType: ErrorTypePermanent,
		},
		{
			name:         "numeric 401 error is permanent",
			err:          errors.New("401: Unauthorized"),
			expectedType: ErrorTypePermanent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyStartupError(tt.err)
			assert.Equal(t, tt.expectedType, result.Type, "Expected %v, got %v", tt.expectedType, result.Type)
		})
	}
}

// TestClassifyStartupError_NetworkError tests that network/timeout errors are classified as transient
func TestClassifyStartupError_NetworkError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
	}{
		{
			name:         "timeout error is transient",
			err:          errors.New("timeout"),
			expectedType: ErrorTypeTransient,
		},
		{
			name:         "connection refused error is transient",
			err:          errors.New("connection refused"),
			expectedType: ErrorTypeTransient,
		},
		{
			name:         "dns lookup error is transient",
			err:          errors.New("lookup failed: no such host"),
			expectedType: ErrorTypeTransient,
		},
		{
			name:         "network timeout error is transient",
			err:          errors.New("network timeout"),
			expectedType: ErrorTypeTransient,
		},
		{
			name:         "connection reset error is transient",
			err:          errors.New("connection reset by peer"),
			expectedType: ErrorTypeTransient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyStartupError(tt.err)
			assert.Equal(t, tt.expectedType, result.Type, "Expected %v, got %v", tt.expectedType, result.Type)
		})
	}
}

// TestClassifyStartupError_NonJSONResponse tests that "invalid character 'e'" is classified as transient
func TestClassifyStartupError_NonJSONResponse(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
	}{
		{
			name:         "invalid character 'e' looking for beginning of value is transient",
			err:          errors.New("invalid character 'e' looking for beginning of value"),
			expectedType: ErrorTypeTransient,
		},
		{
			name:         "other JSON parse errors are transient",
			err:          errors.New("invalid character '{' looking for beginning of value"),
			expectedType: ErrorTypeTransient,
		},
		{
			name:         "syntax error in JSON is transient",
			err:          errors.New("invalid character 't' looking for beginning of value"),
			expectedType: ErrorTypeTransient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyStartupError(tt.err)
			assert.Equal(t, tt.expectedType, result.Type, "Expected %v, got %v", tt.expectedType, result.Type)
		})
	}
}

// TestClassifyStartupError_RateLimitError tests that rate limit errors are classified as transient
func TestClassifyStartupError_RateLimitError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
	}{
		{
			name:         "status 429 is rate limit (transient)",
			err:          errors.New("429 Too Many Requests"),
			expectedType: ErrorTypeTransient,
		},
		{
			name:         "rate limit exceeded is transient",
			err:          errors.New("rate limit exceeded"),
			expectedType: ErrorTypeTransient,
		},
		{
			name:         "too many requests is transient",
			err:          errors.New("too many requests"),
			expectedType: ErrorTypeTransient,
		},
		{
			name:         "ratelimit error is transient",
			err:          errors.New("ratelimit error"),
			expectedType: ErrorTypeTransient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyStartupError(tt.err)
			assert.Equal(t, tt.expectedType, result.Type, "Expected %v, got %v", tt.expectedType, result.Type)
		})
	}
}

// TestValidateToken_EmptyToken tests that empty tokens are rejected
func TestValidateToken_EmptyToken(t *testing.T) {
	err := validateToken("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token cannot be empty")
}

// TestValidateToken_WhitespaceToken tests that tokens with leading/trailing whitespace are rejected
func TestValidateToken_WhitespaceToken(t *testing.T) {
	longToken := "MTE2MTQwNDA2OTE5NjMxMDQ4OA.XXXXX.yyyy1234567890123456789012345678901234567890" // 81 chars, meets format/length requirements (5.5.10+ format)
	tests := []struct {
		name        string
		token       string
		expectError bool
	}{
		{
			name:        "token with leading space",
			token:       " " + longToken,
			expectError: true,
		},
		{
			name:        "token with trailing space",
			token:       longToken + " ",
			expectError: true,
		},
		{
			name:        "token with leading and trailing space",
			token:       " " + longToken + " ",
			expectError: true,
		},
		{
			name:        "valid token with no spaces",
			token:       longToken,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateToken(tt.token)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateToken_ValidToken tests that valid token formats are accepted
func TestValidateToken_ValidToken(t *testing.T) {
	validTokens := []string{
		"MTIzNDU2Nzg5MDEyMzQ1Njc4OTA.X2FiY2RlZmdo.SlpMS1NOVFVWeF9ZYWRtcG9yc3R1dnd4eXphYmNkZWZnaGlqa2xtbmpvcHFyc3R1", // 90+ chars (meets format and length)
		"QUJDREVGQUJDREUxMlNDQktMQTc4OTA.b2FiY2RlZmd.aGltbm9waXFBQUJDREVGQUJCREFCTUVHUmNpd293",                      // 85+ chars (meets format and length)
		"MTExMTExMTExMTExMTEyMzQ1Njc4OTA.QUJDREVGQUE.c29tZWxvbmd0b2tlbnNlY3Rpb25mb3J0ZXN0aW5neXo",                   // 87+ chars (meets format and length)
	}

	for i, token := range validTokens {
		t.Run(fmt.Sprintf("Valid token %d", i+1), func(t *testing.T) {
			err := validateToken(token)
			assert.NoError(t, err, fmt.Sprintf("Valid token (length: %d) should pass validation", len(token)))
		})
	}
}

// TestValidateToken_TooShort tests that short tokens are rejected
func TestValidateToken_TooShort(t *testing.T) {
	shortTokens := []string{
		"",             // empty (should trigger "cannot be empty" first)
		"MTIzND",       // very short token
		"MTIzNDU2Nzg5", // too short still (<59 chars)
		"123",          // just numbers (3 chars)
		"A",            // single char (1 char)
	}

	for i, token := range shortTokens {
		t.Run(fmt.Sprintf("Too short token %d", i+1), func(t *testing.T) {
			err := validateToken(token)
			assert.Error(t, err, "Token '%s' should be rejected", token)
			// Check that the error is either "too short" or "cannot be empty"
			if token == "" {
				assert.Contains(t, err.Error(), "token cannot be empty", "Token '%s' should fail with empty error", token)
			} else {
				assert.Contains(t, err.Error(), "token is too short", "Token '%s' should be too short", token)
			}
		})
	}
}

// TestCalculateBackoffWithJitter_Bounded tests that backoff is properly bounded and has jitter
func TestCalculateBackoffWithJitter_Bounded(t *testing.T) {
	t.Run("tests first few iterations for base delays", func(t *testing.T) {
		// Test early values to see that exponential progression increases as expected with small jitter
		result1 := calculateBackoffWithJitter(time.Second, 1, 60*time.Second)
		result2 := calculateBackoffWithJitter(time.Second, 2, 60*time.Second)
		result3 := calculateBackoffWithJitter(time.Second, 3, 60*time.Second)

		// First: expected ~1-1.25s (1s base, +up to 0.25s jitter)
		// Second: expected ~2-2.5s (2s base, +up to 0.5s jitter)
		// Third: expected ~4-5s (4s base, +up to 1s jitter)

		assert.GreaterOrEqual(t, result1, 1*time.Second, "First attempt should be at least 1s")
		assert.LessOrEqual(t, result1, 2*time.Second, "First attempt with jitter should be less than 2s")

		assert.GreaterOrEqual(t, result2, 2*time.Second, "Second attempt should be at least 2s")
		assert.LessOrEqual(t, result2, 3*time.Second, "Second attempt with jitter should be less than 3s")

		assert.GreaterOrEqual(t, result3, 4*time.Second, "Third attempt should be at least 4s")
		// Allow higher bound because of jitter
		assert.LessOrEqual(t, result3, 6*time.Second, "Third attempt with jitter should be less than 6s")
	})

	// Test that high attempts result in capped delays (60 seconds base + up to 25% jitter = up to 75 seconds possible)
	t.Run("backoff properly caps base time with allowance for jitter", func(t *testing.T) {
		// With maximum delay of 60 seconds and jitter of up to 25%, the max possible is effectively 75 seconds
		maxExpected := 60*time.Second + time.Duration(0.25*float64(60*time.Second))
		result := calculateBackoffWithJitter(time.Second, 13, 60*time.Second)
		assert.LessOrEqual(t, result, maxExpected,
			"Result for attempt 13 (%v) should not exceed maximum of %v (60s + 25%% jitter of 60s)", result, maxExpected)
	})

	// With high attempts the same principle holds: the exponential is capped at max and then jitter added
	t.Run("backoff demonstrates capping across multiple later attempts with jitter consideration", func(t *testing.T) {
		maxExpected := 60*time.Second + time.Duration(0.25*float64(60*time.Second)) // 75 seconds maximum
		for attempt := 10; attempt <= 15; attempt++ {
			result := calculateBackoffWithJitter(time.Second, attempt, 60*time.Second)
			assert.LessOrEqual(t, result, maxExpected,
				"Result for attempt %d (%v) should not exceed maximum of %v",
				attempt, result, maxExpected)
		}
	})

	// Test that there's actual jitter happening by comparing multiple runs of same attempt
	t.Run("backoff produces different values showing jitter", func(t *testing.T) {
		const testIterations = 10
		const baseDelay = time.Second
		const attempt = 2 // Use lower attempt to avoid overflow issues
		const maxDelay = 60 * time.Second

		results := make([]time.Duration, testIterations)
		for i := 0; i < testIterations; i++ {
			results[i] = calculateBackoffWithJitter(baseDelay, attempt, maxDelay)
		}

		// Calculate whether there are multiple non-identical durations
		uniqueValues := make(map[time.Duration]bool)
		for _, d := range results {
			uniqueValues[d] = true
		}

		// Verify that we had variation (there are multiple different values, showing jitter occurred)
		assert.GreaterOrEqual(t, len(uniqueValues), 2, "Jitter should produce multiple different values over multiple calls")

		// Additional check that the values are within reasonable range for attempt 2
		// Expected: 1s * 2^1 = 2s base, then add up to 25% jitter = 0.5s, so max should be ~2.5s
		expectedBase := baseDelay * (1 << uint(attempt-1))                      // 1s * 2 = 2s
		maxPossible := expectedBase + time.Duration(float64(expectedBase)*0.25) // 2s + 0.5s = 2.5s for 25% jitter
		if maxPossible > maxDelay {
			maxPossible = maxDelay
		}

		for _, val := range results {
			assert.GreaterOrEqual(t, val, expectedBase, "Value %v should be >= base %v", val, expectedBase)
			assert.LessOrEqual(t, val, maxPossible, "Value %v should be <= maximum possible %v", val, maxPossible)
		}
	})
}

// TestValidateToken_FormatValidation tests the internal regular expression validation
func TestValidateToken_FormatValidation(t *testing.T) {
	// These tokens meet format requirements: [at least 5 chars].[at least 5 chars].[at least 10 chars] with allowed characters [A-Za-z0-9_-]
	// And are long enough to pass length validation (>= 59 chars)
	longValidFormats := []string{
		"MTIzNDU2Nzg5MDEyMzQ1Njc4OTA.X2FiY2RlZmdo.SlpMS1NOVFVWeF9ZYWRtcG9yc3R1dnd4eXphYmNkZWZnaGlqa2xtbmpvcHFyc3R1", // 93 chars
		"QUJDREVGQUJDREUxMlNDQktMQTc4OTA.b2FiY2RlZmd.aGltbm9waXFBQUJDREVGQUJCREFCTUVHUmNpd293",                      // 87 chars
		"MTExMTExMTExMTExMTEyMzQ1Njc4OTA.QUJDREVGQUE.c29tZWxvbmd0b2tlbnNlY3Rpb25mb3J0ZXN0aW5neXo",                   // 89 chars
	}

	t.Run("valid format tokens pass validation", func(t *testing.T) {
		for _, token := range longValidFormats {
			err := validateToken(token)
			if !assert.NoError(t, err, "Valid token format should pass validation: %s (length: %d)", token, len(token)) {
				t.Logf("Actual error: %v", err)
			}
		}
	})

	// Test tokens that are long enough (>59 chars) but have invalid characters in format
	t.Run("invalid format tokens with sufficient length are correctly detected", func(t *testing.T) {
		// Token with invalid character (not permitted by regex: [A-Za-z0-9_.-])
		invalidCharToken := "VALIDTOKEN123.VALIDSECTOK.VALIDTHIRDP123ANOTHER123INVALIDCHAR+" +
			"MOREVALIDCHARACTERS12345XYZ" // Total >59 chars with an invalid character "+"

		err := validateToken(invalidCharToken)
		assert.Error(t, err, "Token with invalid character (+) should fail format validation")
		assert.Contains(t, err.Error(), "does not match expected Discord token format",
			"Token with invalid character should fail format validation")

		// Another instance with special character
		anotherInvalidToken := "VALIDTOKEN.VALIDPART.VALIDTHIRD" +
			"SOMELONGADDRESSTOENSURELENGTHABOVE59charsbutCONTAINS$char"

		err2 := validateToken(anotherInvalidToken)
		assert.Error(t, err2, "Token with invalid character ($) should fail format validation")
		assert.Contains(t, err2.Error(), "does not match expected Discord token format",
			"Token with invalid character should fail format validation")
	})
}
