package bot

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// validateToken validates the Discord bot token format and content
func validateToken(token string) error {
	// Check if token is empty
	if token == "" {
		return errors.New("token cannot be empty")
	}

	// Check for leading/trailing whitespace
	trimmedToken := strings.TrimSpace(token)
	if len(token) != len(trimmedToken) {
		return errors.New("token cannot have leading or trailing whitespace")
	}

	// Check for any whitespace characters within the token
	for _, char := range token {
		if unicode.IsSpace(char) {
			return errors.New("token cannot contain whitespace characters")
		}
	}

	// Check for newlines and carriage returns (and other control characters that shouldn't be in a token)
	for _, ch := range token {
		if ch == '\n' || ch == '\r' || ch == '\t' || (ch >= 0 && ch <= 31 && ch != '\t') {
			return errors.New("token cannot contain newline, carriage return, tab, or other control characters")
		}
	}

	// Check for quotes in token
	for _, ch := range token {
		if ch == '"' || ch == '\'' {
			return errors.New("token cannot contain quotation marks")
		}
	}

	// Check for reasonable length - Discord tokens are typically longer than 59 characters
	// Discord's minimum token length is approximately 59 characters in practice
	if len(token) < 59 {
		return fmt.Errorf("token is too short (must be 59+ characters): got %d characters", len(token))
	}

	// Check for maximum length (to prevent excessive token lengths or accidental file content)
	if len(token) > 1000 {
		return fmt.Errorf("token is too long (maximum 1000 characters): got %d characters", len(token))
	}

	// Check that the token matches the expected pattern:
	// Typically consists of base64-like characters (A-Z, a-z, 0-9, _, -) and dots, commonly in three parts separated by dots
	tokenPattern := regexp.MustCompile(`^[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{10,}$`)
	if !tokenPattern.MatchString(token) {
		return errors.New("token does not match expected Discord token format")
	}

	return nil
}
