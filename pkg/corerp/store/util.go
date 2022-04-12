// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package store

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

var (
	ErrInvalidKey = errors.New("key includes invalid character")
)

var escapedStorageKeys = []string{
	":00", ":01", ":02", ":03", ":04", ":05", ":06", ":07", ":08", ":09", ":0A", ":0B", ":0C", ":0D", ":0E", ":0F",
	":10", ":11", ":12", ":13", ":14", ":15", ":16", ":17", ":18", ":19", ":1A", ":1B", ":1C", ":1D", ":1E", ":1F",
	":20", ":21", ":22", ":23", ":24", ":25", ":26", ":27", ":28", ":29", ":2A", ":2B", ":2C", ":2D", ":2E", ":2F",
	"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", ":3A", ":3B", ":3C", ":3D", ":3E", ":3F",
	":40", "A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O",
	"P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z", ":5B", ":5C", ":5D", ":5E", ":5F",
	":60", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o",
	"p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", ":7B", ":7C", ":7D", ":7E", ":7F",
}

const (
	keyDelimiter = "-"
)

// NormalizeLetterOrDigitToUpper normalizes the value to only letter or digit to upper invariant.
func NormalizeLetterOrDigitToUpper(s string) string {
	if s == "" {
		return s
	}

	sb := strings.Builder{}
	for _, ch := range s {
		if unicode.IsDigit(ch) || unicode.IsLetter(ch) {
			sb.WriteRune(ch)
		}
	}

	return strings.ToUpper(sb.String())
}

// NomalizeSubscriptionID normalizes subscription id.
func NomalizeSubscriptionID(subscriptionID string) string {
	return NormalizeLetterOrDigitToUpper(subscriptionID)
}

// NormalizeLocation normalizes location. e.g. "West US" -> "WESTUS"
func NormalizeLocation(location string) string {
	return NormalizeLocation(location)
}

func EscapedStorageKey(key string) string {
	sb := strings.Builder{}
	for _, ch := range key {
		if ch < 128 {
			sb.WriteString(escapedStorageKeys[ch])
		} else if unicode.IsDigit(ch) || unicode.IsLetter(ch) {
			sb.WriteRune(ch)
		} else if ch < 0x100 {
			sb.WriteRune(':')
			sb.WriteString(fmt.Sprintf("%02d", ch))
		} else {
			sb.WriteRune(':')
			sb.WriteRune(':')
			sb.WriteString(fmt.Sprintf("%04d", ch))
		}
	}
	return sb.String()
}

func CombineStorageKeys(keys ...string) (string, error) {
	for _, key := range keys {
		if strings.Contains(key, keyDelimiter) {
			return "", ErrInvalidKey
		}
	}

	return strings.Join(keys, keyDelimiter), nil
}
