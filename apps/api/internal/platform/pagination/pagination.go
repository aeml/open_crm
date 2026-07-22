package pagination

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	DefaultPage   = 1
	MaxPageSize   = 100
	MaxPageOffset = 50_000
)

var ErrInvalid = errors.New("invalid pagination")

type Page struct {
	Number int
	Size   int
	Offset int
}

// Parse validates caller-supplied page values. Missing values use the defaults,
// while malformed, non-positive, oversized, and excessively deep pages fail
// explicitly instead of silently changing the request.
func Parse(pageValue, pageSizeValue string, defaultPageSize int) (Page, error) {
	page, err := parseValue(pageValue, DefaultPage)
	if err != nil {
		return Page{}, err
	}
	pageSize, err := parseValue(pageSizeValue, defaultPageSize)
	if err != nil {
		return Page{}, err
	}
	return validate(page, pageSize)
}

// Normalize protects service callers that do not enter through HTTP. Zero or
// negative values retain the historical default behavior; positive values must
// satisfy the same hard bounds as public requests.
func Normalize(page, pageSize, defaultPageSize int) (Page, error) {
	if page <= 0 {
		page = DefaultPage
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	return validate(page, pageSize)
}

func parseValue(value string, fallback int) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%w: page values must be positive integers", ErrInvalid)
	}
	return parsed, nil
}

func validate(page, pageSize int) (Page, error) {
	if page <= 0 || pageSize <= 0 || pageSize > MaxPageSize {
		return Page{}, fmt.Errorf("%w: page size must be between 1 and %d", ErrInvalid, MaxPageSize)
	}
	// This comparison avoids overflowing int when a service caller supplies an
	// extreme page number.
	if page-1 > MaxPageOffset/pageSize {
		return Page{}, fmt.Errorf("%w: page offset must not exceed %d records", ErrInvalid, MaxPageOffset)
	}
	offset := (page - 1) * pageSize
	return Page{Number: page, Size: pageSize, Offset: offset}, nil
}
