package handlers

import (
	"errors"
	"strings"
	"time"
)

var (
	errSerialMissing    = errors.New("serial missing")
	errSerialUsed       = errors.New("serial used")
	errRequiredCustomer = errors.New("required customer")
)

func parseDate(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}

	return &parsed, nil
}

func parseFlexibleDate(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return &parsed, nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}

	normalized := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC)
	return &normalized, nil
}

func normalizeSerial(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ".", "-")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "item"
	}
	return value
}
