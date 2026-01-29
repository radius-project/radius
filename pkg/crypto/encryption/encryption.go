/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package encryption

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// KeySize is the required size for ChaCha20-Poly1305 keys (256 bits).
	KeySize = chacha20poly1305.KeySize

	// NonceSize is the size of the nonce for ChaCha20-Poly1305.
	NonceSize = chacha20poly1305.NonceSize
)

var (
	// ErrInvalidKeySize is returned when the encryption key is not the correct size.
	ErrInvalidKeySize = errors.New("encryption key must be 32 bytes (256 bits)")

	// ErrEncryptionFailed is returned when encryption fails.
	ErrEncryptionFailed = errors.New("encryption failed")

	// ErrDecryptionFailed is returned when decryption fails.
	ErrDecryptionFailed = errors.New("decryption failed")

	// ErrInvalidEncryptedData is returned when the encrypted data format is invalid.
	ErrInvalidEncryptedData = errors.New("invalid encrypted data format")

	// ErrEmptyPlaintext is returned when attempting to encrypt empty data.
	ErrEmptyPlaintext = errors.New("plaintext cannot be empty")

	// ErrAssociatedDataMismatch is returned when the associated data provided during
	// decryption does not match what was used during encryption.
	ErrAssociatedDataMismatch = errors.New("associated data mismatch")
)

// EncryptedData represents the structure for storing encrypted data.
// It contains the base64-encoded ciphertext and nonce, plus optional associated data hash.
type EncryptedData struct {
	// Version is the key version used for encryption.
	// This allows decryption to use the correct key when multiple versions exist.
	Version int `json:"version,omitempty"`
	// Encrypted contains the base64-encoded ciphertext.
	Encrypted string `json:"encrypted"`
	// Nonce contains the base64-encoded nonce used for encryption.
	Nonce string `json:"nonce"`
	// AD contains a hash of the associated data used during encryption (optional).
	// This is stored for verification purposes - the actual AD must be provided during decryption.
	// The hash allows detection of AD mismatches without exposing the AD value.
	AD string `json:"ad,omitempty"`
}

// Encryptor provides methods for encrypting and decrypting data using ChaCha20-Poly1305.
type Encryptor struct {
	aead       cipher.AEAD
	keyVersion int
}

// NewEncryptor creates a new Encryptor with the provided 256-bit key.
// Returns an error if the key is not exactly 32 bytes.
// The key version defaults to 0 (unversioned). Use NewEncryptorWithVersion for versioned keys.
func NewEncryptor(key []byte) (*Encryptor, error) {
	return NewEncryptorWithVersion(key, 0)
}

// NewEncryptorWithVersion creates a new Encryptor with the provided key and version.
// The version is stored in encrypted data to enable decryption with the correct key.
func NewEncryptorWithVersion(key []byte, version int) (*Encryptor, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	return &Encryptor{aead: aead, keyVersion: version}, nil
}

// Encrypt encrypts the plaintext using ChaCha20-Poly1305 with Associated Data (AD).
// The AD provides authentication for contextual data (like resource ID or field path) without
// encrypting it. This binds the ciphertext to its context, preventing an attacker from
// moving encrypted values between different resources or fields.
//
// The AD is authenticated but NOT encrypted - it must be provided again during decryption.
// A hash of the AD is stored in the encrypted data structure to allow early detection of mismatches.
//
// Example AD values:
//   - Resource ID: "/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test"
//   - Field path: "credentials.password"
//   - Combined: resourceID + ":" + fieldPath
//
// Pass nil for associatedData if no context binding is needed (not recommended for sensitive data).
func (e *Encryptor) Encrypt(plaintext []byte, associatedData []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, ErrEmptyPlaintext
	}

	// Generate a unique nonce for this encryption operation
	nonce, err := generateNonce(e.aead.NonceSize())
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate nonce: %v", ErrEncryptionFailed, err)
	}

	// Encrypt the plaintext with associated data
	// The AD is authenticated (included in the auth tag) but not encrypted
	ciphertext := e.aead.Seal(nil, nonce, plaintext, associatedData)

	// Create the encrypted data structure
	encryptedData := EncryptedData{
		Version:   e.keyVersion,
		Encrypted: base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
	}

	// Store a hash of the AD if provided (for verification during decryption)
	if len(associatedData) > 0 {
		encryptedData.AD = hashAD(associatedData)
	}

	// Marshal to JSON
	result, err := json.Marshal(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to marshal encrypted data: %v", ErrEncryptionFailed, err)
	}

	return result, nil
}

// Decrypt decrypts the data that was encrypted using the Encrypt method.
// The associatedData must match what was provided during encryption; if the AD
// was used during encryption, it must be provided here for successful decryption.
// The input should be JSON-encoded EncryptedData.
func (e *Encryptor) Decrypt(data []byte, associatedData []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidEncryptedData
	}

	// Parse the encrypted data structure
	var encryptedData EncryptedData
	if err := json.Unmarshal(data, &encryptedData); err != nil {
		return nil, fmt.Errorf("%w: failed to parse encrypted data: %v", ErrInvalidEncryptedData, err)
	}

	// Verify AD hash matches if AD was used during encryption
	if encryptedData.AD != "" {
		if len(associatedData) == 0 {
			return nil, fmt.Errorf("%w: encrypted data requires associated data but none provided", ErrAssociatedDataMismatch)
		}
		if hashAD(associatedData) != encryptedData.AD {
			return nil, fmt.Errorf("%w: provided associated data does not match", ErrAssociatedDataMismatch)
		}
	}

	// Decode the base64-encoded ciphertext
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedData.Encrypted)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode ciphertext: %v", ErrInvalidEncryptedData, err)
	}

	// Decode the base64-encoded nonce
	nonce, err := base64.StdEncoding.DecodeString(encryptedData.Nonce)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode nonce: %v", ErrInvalidEncryptedData, err)
	}

	// Validate nonce size
	if len(nonce) != e.aead.NonceSize() {
		return nil, fmt.Errorf("%w: invalid nonce size", ErrInvalidEncryptedData)
	}

	// Decrypt the ciphertext with the same associated data
	plaintext, err := e.aead.Open(nil, nonce, ciphertext, associatedData)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}

// EncryptString encrypts a string with associated data and returns the JSON-encoded encrypted data as a string.
func (e *Encryptor) EncryptString(plaintext string, associatedData []byte) (string, error) {
	encrypted, err := e.Encrypt([]byte(plaintext), associatedData)
	if err != nil {
		return "", err
	}
	return string(encrypted), nil
}

// DecryptString decrypts the JSON-encoded encrypted data with associated data and returns the original string.
func (e *Encryptor) DecryptString(data string, associatedData []byte) (string, error) {
	decrypted, err := e.Decrypt([]byte(data), associatedData)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}

// hashAD creates a truncated SHA-256 hash of the associated data for storage.
// This allows verification that the correct AD is provided during decryption
// without storing the actual AD value.
func hashAD(ad []byte) string {
	hash := sha256.Sum256(ad)
	// Use first 16 bytes (128 bits) - sufficient for verification, saves storage
	return base64.StdEncoding.EncodeToString(hash[:16])
}

// generateNonce generates a cryptographically secure random nonce.
func generateNonce(size int) ([]byte, error) {
	nonce := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return nonce, nil
}

// IsEncryptedData checks if the given data appears to be in the encrypted data format.
// It validates that the data is valid JSON with non-empty encrypted and nonce fields,
// and that both fields contain valid base64-encoded data with appropriate nonce size.
func IsEncryptedData(data []byte) bool {
	var encryptedData EncryptedData
	if err := json.Unmarshal(data, &encryptedData); err != nil {
		return false
	}

	if encryptedData.Encrypted == "" || encryptedData.Nonce == "" {
		return false
	}

	// Validate base64 encoding of ciphertext
	if _, err := base64.StdEncoding.DecodeString(encryptedData.Encrypted); err != nil {
		return false
	}

	// Validate base64 encoding and size of nonce
	nonce, err := base64.StdEncoding.DecodeString(encryptedData.Nonce)
	if err != nil {
		return false
	}

	// ChaCha20-Poly1305 nonce must be 12 bytes
	if len(nonce) != NonceSize {
		return false
	}

	return true
}

// GenerateKey generates a new random 256-bit encryption key.
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}
	return key, nil
}

// GetEncryptedDataVersion extracts the key version from encrypted data without decrypting.
// Returns 0 if the version is not present (for backwards compatibility with unversioned data).
func GetEncryptedDataVersion(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, ErrInvalidEncryptedData
	}

	var encryptedData EncryptedData
	if err := json.Unmarshal(data, &encryptedData); err != nil {
		return 0, fmt.Errorf("%w: failed to parse encrypted data: %v", ErrInvalidEncryptedData, err)
	}

	return encryptedData.Version, nil
}
