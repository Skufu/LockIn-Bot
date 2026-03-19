package bot

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// ErrorType represents the type of startup error
type ErrorType int

const (
	// ErrorTypePermanent - Invalid token format, missing config
	ErrorTypePermanent ErrorType = iota
	// ErrorTypeTransient - Network timeout, rate limit, Cloudflare block
	ErrorTypeTransient
	// ErrorTypeUnknown - Non-JSON response, unexpected errors
	ErrorTypeUnknown
)

// isDNSError checks if the error is related to DNS resolution
func isDNSError(err error) bool {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() || strings.Contains(strings.ToLower(urlErr.Error()), "no such host") {
			return true
		}
	}

	// Check for DNS and network errors in various types
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "temporary failure in name resolution") ||
		strings.Contains(errStr, "server misbehaving")
}

// BotStartupError represents a classified error during bot startup
type BotStartupError struct {
	Type       ErrorType
	Message    string
	Original   error
	HTTPStatus int // HTTP status code if applicable (0 if not applicable)
}

// Error implements the error interface
func (e BotStartupError) Error() string {
	if e.HTTPStatus != 0 {
		return fmt.Sprintf("[%s] %s (HTTP %d): %v", e.getTypeString(), e.Message, e.HTTPStatus, e.Original)
	}
	return fmt.Sprintf("[%s] %s: %v", e.getTypeString(), e.Message, e.Original)
}

// Unwrap implements the unwrap interface for error wrapping
func (e BotStartupError) Unwrap() error {
	return e.Original
}

// getTypeString returns a string representation of the error type
func (e BotStartupError) getTypeString() string {
	switch e.Type {
	case ErrorTypePermanent:
		return "PERMANENT"
	case ErrorTypeTransient:
		return "TRANSIENT"
	case ErrorTypeUnknown:
		return "UNKNOWN"
	default:
		return "UNKNOWN"
	}
}

// classifyStartupError analyzes an error and returns a classified BotStartupError
func classifyStartupError(err error) BotStartupError {
	if err == nil {
		return BotStartupError{
			Type:       ErrorTypeUnknown,
			Message:    "no error provided",
			Original:   nil,
			HTTPStatus: 0,
		}
	}

	originalErr := err
	errMsg := err.Error()
	lowerErrMsg := strings.ToLower(errMsg)

	// Parse HTTP status from discordgo's RESTError if present
	httpStatus := 0
	if restErr, ok := err.(*discordgo.RESTError); ok {
		if restErr.Response != nil {
			httpStatus = restErr.Response.StatusCode
		}
	}

	// Check for specific non-JSON response case mentioned in requirements
	if strings.Contains(errMsg, "invalid character 'e' looking for beginning of value") {
		return BotStartupError{
			Type:       ErrorTypeTransient,
			Message:    "non-JSON response from Discord, likely a service interruption or Cloudflare block",
			Original:   originalErr,
			HTTPStatus: httpStatus,
		}
	}

	// Check for authentication/authorization errors (permanent errors)
	authErrors := []string{
		"401", // Unauthorized
		"403", // Forbidden
		"invalid token",
		"unauthorized",
		"forbidden",
		"authentication",
		"token",
	}

	for _, authErr := range authErrors {
		if strings.Contains(lowerErrMsg, strings.ToLower(authErr)) {
			return BotStartupError{
				Type:       ErrorTypePermanent,
				Message:    "invalid Discord token or authentication failure",
				Original:   originalErr,
				HTTPStatus: httpStatus,
			}
		}
	}

	// Check for network-related errors (transient errors)
	networkErrors := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"network",
		"dns",
		"no such host",
		"temporarily unavailable",
	}

	for _, netErr := range networkErrors {
		if strings.Contains(lowerErrMsg, strings.ToLower(netErr)) {
			return BotStartupError{
				Type:       ErrorTypeTransient,
				Message:    "network connectivity issue",
				Original:   originalErr,
				HTTPStatus: httpStatus,
			}
		}
	}

	// Check for rate limit errors (transient errors)
	rateLimitErrors := []string{
		"429",
		"rate limit",
		"too many requests",
		"ratelimit",
	}

	for _, rateErr := range rateLimitErrors {
		if strings.Contains(lowerErrMsg, strings.ToLower(rateErr)) {
			return BotStartupError{
				Type:       ErrorTypeTransient,
				Message:    "rate limited by Discord, can be retried after cooldown",
				Original:   originalErr,
				HTTPStatus: httpStatus,
			}
		}
	}

	// Check for DNS resolution errors
	if isDNSError(err) {
		return BotStartupError{
			Type:       ErrorTypeTransient,
			Message:    "DNS resolution failed",
			Original:   originalErr,
			HTTPStatus: httpStatus,
		}
	}

	// Check for non-JSON/parse errors (transient errors in most cases)
	jsonParseErrors := []string{
		"invalid character",
		"looking for beginning of value",
		"unexpected end of JSON input",
		"cannot unmarshal",
		"syntax error",
		"malformed",
	}

	for _, jsonErr := range jsonParseErrors {
		if strings.Contains(lowerErrMsg, strings.ToLower(jsonErr)) {
			return BotStartupError{
				Type:       ErrorTypeTransient, // Most non-JSON errors from Discord are typically transient issues
				Message:    "response parsing failed (likely non-JSON response from Discord)",
				Original:   originalErr,
				HTTPStatus: httpStatus,
			}
		}
	}

	// Check for missing configuration errors (permanent errors)
	configErrors := []string{
		"missing",
		"not found",
		"required",
		"configuration",
		"env",
	}

	for _, configErr := range configErrors {
		if strings.Contains(lowerErrMsg, strings.ToLower(configErr)) {
			return BotStartupError{
				Type:       ErrorTypePermanent,
				Message:    "missing or invalid configuration",
				Original:   originalErr,
				HTTPStatus: httpStatus,
			}
		}
	}

	// Default to Unknown for unclassified errors
	return BotStartupError{
		Type:       ErrorTypeUnknown,
		Message:    "uncategorized error",
		Original:   originalErr,
		HTTPStatus: httpStatus,
	}
}

// Helper function to wrap errors with classification
func WrapAsBotStartupError(err error) BotStartupError {
	if err == nil {
		return BotStartupError{
			Type:       ErrorTypeUnknown,
			Message:    "nil error passed to WrapAsBotStartupError",
			Original:   nil,
			HTTPStatus: 0,
		}
	}

	// If it's already a BotStartupError, return as-is
	if botErr, ok := err.(BotStartupError); ok {
		return botErr
	}

	return classifyStartupError(err)
}
