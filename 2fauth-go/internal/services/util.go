package services

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// isNumeric mirrors PHP's is_numeric() for the values 2FAuth stores.
func isNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	dot := 0
	for i, r := range s {
		if r == '-' || r == '+' {
			if i != 0 {
				return false
			}
			continue
		}
		if r == 'e' || r == 'E' {
			continue
		}
		if r == '.' {
			dot++
			if dot > 1 {
				return false
			}
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseInt(s string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseFloat(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// sha256Hex returns the hex digest of a byte slice.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
