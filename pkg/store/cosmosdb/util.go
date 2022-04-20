// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/store"
	"github.com/spaolacci/murmur3"
)

const (
	keyDelimiter = "-"

	// StorageKeyTrimPaddingLen is the length of the padding when key is trimed.
	StorageKeyTrimPaddingLen = 17
	// The resource group name storage key length.
	ResourceGroupNameMaxStorageKeyLen = 64
	// The resource identifier storage key limit.
	ResourceIdMaxStorageKeyLen = 157
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

// NormalizeSubscriptionID normalizes subscription id.
func NormalizeSubscriptionID(subscriptionID string) string {
	return NormalizeLetterOrDigitToUpper(subscriptionID)
}

// EscapedStorageKey escapes the storage key.
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

// CombineStroageKeys combines the storage keys.
func CombineStorageKeys(keys ...string) (string, error) {
	for _, key := range keys {
		if strings.Contains(key, keyDelimiter) {
			return "", ErrInvalidKey
		}
	}

	return strings.Join(keys, keyDelimiter), nil
}

// TrimStorageKey trims long storage key. If the length of
func TrimStorageKey(storageKey string, maxLength int) (string, error) {
	if maxLength < StorageKeyTrimPaddingLen {
		return "", &store.ErrInvalid{Message: "storage key is too short"}
	}
	if strings.Contains(storageKey, "|") {
		return "", &store.ErrInvalid{Message: "storage key is not properly encoded"}
	}
	if len(storageKey) > maxLength {
		// Use murmur hash to generate unique key if the length of key exceeds maxLenth
		storageKey = fmt.Sprintf("%s|%16X", storageKey[:(maxLength-StorageKeyTrimPaddingLen)], murmur3.Sum64([]byte(storageKey)))
	}
	return storageKey, nil
}

// NormalizeStorageKey must normalize storagekey.
func NormalizeStorageKey(storageKey string, maxLength int) (string, error) {
	upper := strings.ToUpper(storageKey)
	return TrimStorageKey(EscapedStorageKey(upper), maxLength)
}

// GenerateCosmosDBKey generates the unqiue key the length of which must be less than 255.
func GenerateCosmosDBKey(subsID, resourceGroup, fullyQualifiedType, fullyQualifiedName string) (string, error) {
	unqiueResourceGroup, err := NormalizeStorageKey(resourceGroup, ResourceGroupNameMaxStorageKeyLen)
	if err != nil {
		return "", err
	}
	resourceTypeAndName, err := NormalizeStorageKey(fullyQualifiedType+fullyQualifiedName, ResourceIdMaxStorageKeyLen)
	if err != nil {
		return "", err
	}
	return CombineStorageKeys(NormalizeSubscriptionID(subsID), unqiueResourceGroup, resourceTypeAndName)
}

const (
	subscriptionType  = "subscriptions"
	resourceGroupType = "resourceGroups"
)

// TODO: replace ResourceScope with UCP resource id when we merge it to ucp repo - https://github.com/project-radius/radius/issues/2224
type ResourceScope struct {
	SubscriptionID string
	ResourceGroup  string
}

func NewResourceScope(rootScope string) (*ResourceScope, error) {
	rScope := &ResourceScope{}
	scope := strings.TrimPrefix(rootScope, "/")
	scope = strings.TrimSuffix(scope, "/")
	s := strings.Split(scope, "/")
	if len(s) >= 2 && strings.EqualFold(s[0], subscriptionType) {
		if _, err := uuid.Parse(s[1]); err != nil {
			return nil, err
		}
		rScope.SubscriptionID = strings.ToLower(s[1])
	}
	if len(s) >= 4 && strings.EqualFold(s[2], resourceGroupType) {
		rScope.ResourceGroup = strings.ToLower(s[3])
	}
	return rScope, nil
}

func (r ResourceScope) fullyQualifiedSubscriptionScope() string {
	sb := strings.Builder{}
	if r.SubscriptionID != "" {
		sb.WriteString("/")
		sb.WriteString(subscriptionType)
		sb.WriteString("/")
		sb.WriteString(r.SubscriptionID)
	}
	return strings.ToLower(sb.String())
}
