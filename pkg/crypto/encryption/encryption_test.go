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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewEncryptor(t *testing.T) {
	tests := []struct {
		name    string
		key     []byte
		wantErr error
	}{
		{
			name:    "valid-32-byte-key",
			key:     make([]byte, 32),
			wantErr: nil,
		},
		{
			name:    "invalid-key-too-short",
			key:     make([]byte, 16),
			wantErr: ErrInvalidKeySize,
		},
		{
			name:    "invalid-key-too-long",
			key:     make([]byte, 64),
			wantErr: ErrInvalidKeySize,
		},
		{
			name:    "invalid-empty-key",
			key:     []byte{},
			wantErr: ErrInvalidKeySize,
		},
		{
			name:    "invalid-nil-key",
			key:     nil,
			wantErr: ErrInvalidKeySize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := NewEncryptor(tt.key)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Nil(t, enc)
			} else {
				require.NoError(t, err)
				require.NotNil(t, enc)
			}
		})
	}
}

func TestEncrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	tests := []struct {
		name      string
		plaintext []byte
		wantErr   error
	}{
		{
			name:      "encrypt-simple-text",
			plaintext: []byte("hello world"),
			wantErr:   nil,
		},
		{
			name:      "encrypt-json-data",
			plaintext: []byte(`{"password": "secret123", "token": "abc-xyz"}`),
			wantErr:   nil,
		},
		{
			name:      "encrypt-binary-data",
			plaintext: []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd},
			wantErr:   nil,
		},
		{
			name:      "encrypt-long-text",
			plaintext: make([]byte, 10000),
			wantErr:   nil,
		},
		{
			name:      "encrypt-empty-plaintext",
			plaintext: []byte{},
			wantErr:   ErrEmptyPlaintext,
		},
		{
			name:      "encrypt-nil-plaintext",
			plaintext: nil,
			wantErr:   ErrEmptyPlaintext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := enc.Encrypt(tt.plaintext, nil)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Nil(t, encrypted)
			} else {
				require.NoError(t, err)
				require.NotNil(t, encrypted)

				// Verify the encrypted data is valid JSON with expected structure
				var encData EncryptedData
				err = json.Unmarshal(encrypted, &encData)
				require.NoError(t, err)
				require.NotEmpty(t, encData.Encrypted)
				require.NotEmpty(t, encData.Nonce)
			}
		})
	}
}

func TestDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	// Create valid encrypted data for testing
	validPlaintext := []byte("secret data")
	validEncrypted, err := enc.Encrypt(validPlaintext, nil)
	require.NoError(t, err)

	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{
			name:    "decrypt-valid-data",
			data:    validEncrypted,
			wantErr: nil,
		},
		{
			name:    "decrypt-empty-data",
			data:    []byte{},
			wantErr: ErrInvalidEncryptedData,
		},
		{
			name:    "decrypt-nil-data",
			data:    nil,
			wantErr: ErrInvalidEncryptedData,
		},
		{
			name:    "decrypt-invalid-json",
			data:    []byte("not json"),
			wantErr: ErrInvalidEncryptedData,
		},
		{
			name:    "decrypt-missing-encrypted-field",
			data:    []byte(`{"nonce": "dGVzdA=="}`),
			wantErr: ErrInvalidEncryptedData,
		},
		{
			name:    "decrypt-missing-nonce-field",
			data:    []byte(`{"encrypted": "dGVzdA=="}`),
			wantErr: ErrInvalidEncryptedData,
		},
		{
			name:    "decrypt-invalid-base64-ciphertext",
			data:    []byte(`{"encrypted": "not-valid-base64!!!", "nonce": "dGVzdA=="}`),
			wantErr: ErrInvalidEncryptedData,
		},
		{
			name:    "decrypt-invalid-base64-nonce",
			data:    []byte(`{"encrypted": "dGVzdA==", "nonce": "not-valid-base64!!!"}`),
			wantErr: ErrInvalidEncryptedData,
		},
		{
			name:    "decrypt-wrong-nonce-size",
			data:    []byte(`{"encrypted": "dGVzdA==", "nonce": "dGVzdA=="}`),
			wantErr: ErrInvalidEncryptedData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decrypted, err := enc.Decrypt(tt.data, nil)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, validPlaintext, decrypted)
			}
		})
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	testCases := []struct {
		name      string
		plaintext []byte
	}{
		{
			name:      "simple-text",
			plaintext: []byte("hello world"),
		},
		{
			name:      "json-secret",
			plaintext: []byte(`{"password": "super-secret-password", "apiKey": "xyz-123-abc"}`),
		},
		{
			name:      "unicode-text",
			plaintext: []byte("Hello 世界! 🔐"),
		},
		{
			name:      "binary-data",
			plaintext: []byte{0x00, 0x01, 0x02, 0x03, 0xff, 0xfe, 0xfd, 0xfc},
		},
		{
			name:      "large-data",
			plaintext: make([]byte, 65536),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Encrypt
			encrypted, err := enc.Encrypt(tc.plaintext, nil)
			require.NoError(t, err)
			require.NotEqual(t, tc.plaintext, encrypted, "encrypted data should differ from plaintext")

			// Decrypt
			decrypted, err := enc.Decrypt(encrypted, nil)
			require.NoError(t, err)
			require.Equal(t, tc.plaintext, decrypted, "decrypted data should match original plaintext")
		})
	}
}

func TestEncryptStringDecryptString(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	testCases := []string{
		"simple password",
		"API-KEY-12345",
		`{"token": "secret"}`,
		"Unicode: 日本語 🔑",
	}

	for _, plaintext := range testCases {
		t.Run(plaintext, func(t *testing.T) {
			encrypted, err := enc.EncryptString(plaintext, nil)
			require.NoError(t, err)
			require.NotEqual(t, plaintext, encrypted)

			decrypted, err := enc.DecryptString(encrypted, nil)
			require.NoError(t, err)
			require.Equal(t, plaintext, decrypted)
		})
	}
}

func TestUniqueNoncesPerEncryption(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	plaintext := []byte("same plaintext")
	nonces := make(map[string]bool)

	// Encrypt the same plaintext multiple times
	for i := 0; i < 100; i++ {
		encrypted, err := enc.Encrypt(plaintext, nil)
		require.NoError(t, err)

		var encData EncryptedData
		err = json.Unmarshal(encrypted, &encData)
		require.NoError(t, err)

		// Each nonce should be unique
		require.False(t, nonces[encData.Nonce], "nonce should be unique for each encryption")
		nonces[encData.Nonce] = true
	}
}

func TestDifferentKeysCannotDecrypt(t *testing.T) {
	key1, err := GenerateKey()
	require.NoError(t, err)

	key2, err := GenerateKey()
	require.NoError(t, err)

	enc1, err := NewEncryptor(key1)
	require.NoError(t, err)

	enc2, err := NewEncryptor(key2)
	require.NoError(t, err)

	plaintext := []byte("secret message")

	// Encrypt with key1
	encrypted, err := enc1.Encrypt(plaintext, nil)
	require.NoError(t, err)

	// Try to decrypt with key2 - should fail
	_, err = enc2.Decrypt(encrypted, nil)
	require.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestGenerateKey(t *testing.T) {
	key1, err := GenerateKey()
	require.NoError(t, err)
	require.Len(t, key1, KeySize)

	key2, err := GenerateKey()
	require.NoError(t, err)
	require.Len(t, key2, KeySize)

	// Keys should be different
	require.NotEqual(t, key1, key2)
}

func TestIsEncryptedData(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	encrypted, err := enc.Encrypt([]byte("test"), nil)
	require.NoError(t, err)

	// Valid 12-byte nonce encoded in base64 for manual test cases
	validNonceBase64 := "AAAAAAAAAAAAAAAA" // 12 bytes of zeros in base64

	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "valid-encrypted-data",
			data: encrypted,
			want: true,
		},
		{
			name: "valid-format-manual",
			data: []byte(`{"encrypted": "YWJjZGVm", "nonce": "` + validNonceBase64 + `"}`),
			want: true,
		},
		{
			name: "invalid-base64-encrypted",
			data: []byte(`{"encrypted": "not-valid-base64!!!", "nonce": "` + validNonceBase64 + `"}`),
			want: false,
		},
		{
			name: "invalid-base64-nonce",
			data: []byte(`{"encrypted": "YWJjZGVm", "nonce": "not-valid-base64!!!"}`),
			want: false,
		},
		{
			name: "invalid-nonce-size",
			data: []byte(`{"encrypted": "YWJjZGVm", "nonce": "YWJj"}`), // "abc" = 3 bytes, not 12
			want: false,
		},
		{
			name: "missing-encrypted-field",
			data: []byte(`{"nonce": "` + validNonceBase64 + `"}`),
			want: false,
		},
		{
			name: "missing-nonce-field",
			data: []byte(`{"encrypted": "YWJjZGVm"}`),
			want: false,
		},
		{
			name: "empty-fields",
			data: []byte(`{"encrypted": "", "nonce": ""}`),
			want: false,
		},
		{
			name: "invalid-json",
			data: []byte("not json"),
			want: false,
		},
		{
			name: "empty-data",
			data: []byte{},
			want: false,
		},
		{
			name: "nil-data",
			data: nil,
			want: false,
		},
		{
			name: "plain-text",
			data: []byte("just plain text"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEncryptedData(tt.data)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestTamperDetection(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	plaintext := []byte("sensitive data")
	encrypted, err := enc.Encrypt(plaintext, nil)
	require.NoError(t, err)

	// Parse the encrypted data
	var encData EncryptedData
	err = json.Unmarshal(encrypted, &encData)
	require.NoError(t, err)

	// Tamper with the ciphertext by modifying the base64 string
	// We'll change the first character
	tampered := encData.Encrypted
	if tampered[0] == 'A' {
		tampered = "B" + tampered[1:]
	} else {
		tampered = "A" + tampered[1:]
	}
	encData.Encrypted = tampered

	tamperedJSON, err := json.Marshal(encData)
	require.NoError(t, err)

	// Decryption should fail due to authentication tag mismatch
	_, err = enc.Decrypt(tamperedJSON, nil)
	require.Error(t, err)
}

// Tests for Associated Data (AD) functionality

func TestEncryptWithAssociatedData(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	plaintext := []byte("secret data")
	resourceID := "/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test"
	fieldPath := "credentials.password"
	ad := []byte(resourceID + ":" + fieldPath)

	// Encrypt with AD
	encrypted, err := enc.Encrypt(plaintext, ad)
	require.NoError(t, err)

	// Verify AD hash is stored
	var encData EncryptedData
	err = json.Unmarshal(encrypted, &encData)
	require.NoError(t, err)
	require.NotEmpty(t, encData.AD, "AD hash should be stored")

	// Decrypt with same AD should succeed
	decrypted, err := enc.Decrypt(encrypted, ad)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func TestDecryptWithWrongAssociatedData(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	plaintext := []byte("secret data")
	ad1 := []byte("/resource/1:password")
	ad2 := []byte("/resource/2:password")

	// Encrypt with AD1
	encrypted, err := enc.Encrypt(plaintext, ad1)
	require.NoError(t, err)

	// Decrypt with different AD should fail with mismatch error
	_, err = enc.Decrypt(encrypted, ad2)
	require.ErrorIs(t, err, ErrAssociatedDataMismatch)
}

func TestDecryptWithMissingAssociatedData(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	plaintext := []byte("secret data")
	ad := []byte("/resource/1:password")

	// Encrypt with AD
	encrypted, err := enc.Encrypt(plaintext, ad)
	require.NoError(t, err)

	// Decrypt without AD when AD was used should fail
	_, err = enc.Decrypt(encrypted, nil)
	require.ErrorIs(t, err, ErrAssociatedDataMismatch)
}

func TestEncryptWithoutAssociatedDataDecryptWithAD(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	plaintext := []byte("secret data")

	// Encrypt without AD
	encrypted, err := enc.Encrypt(plaintext, nil)
	require.NoError(t, err)

	// Verify no AD hash is stored
	var encData EncryptedData
	err = json.Unmarshal(encrypted, &encData)
	require.NoError(t, err)
	require.Empty(t, encData.AD, "AD hash should not be stored when no AD provided")

	// Decrypt without AD should succeed
	decrypted, err := enc.Decrypt(encrypted, nil)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)

	// Decrypt with AD when no AD was used - AEAD will fail because the auth tag won't match
	_, err = enc.Decrypt(encrypted, []byte("unexpected-ad"))
	require.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestAssociatedDataPreventsContextSwitch(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	enc, err := NewEncryptor(key)
	require.NoError(t, err)

	// Simulate encrypting a password for resource1
	password := []byte("super-secret-password")
	resource1AD := []byte("/resource/1:password")
	resource2AD := []byte("/resource/2:password")

	// Encrypt password for resource1
	encryptedForResource1, err := enc.Encrypt(password, resource1AD)
	require.NoError(t, err)

	// Attacker tries to use this encrypted value for resource2
	// This should fail because the AD is different
	_, err = enc.Decrypt(encryptedForResource1, resource2AD)
	require.Error(t, err, "should not be able to decrypt with different resource context")
}

// Tests for versioned key support

func TestNewEncryptorWithVersion(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	tests := []struct {
		name    string
		key     []byte
		version int
		wantErr error
	}{
		{
			name:    "valid-key-with-version-1",
			key:     key,
			version: 1,
			wantErr: nil,
		},
		{
			name:    "valid-key-with-version-2",
			key:     key,
			version: 2,
			wantErr: nil,
		},
		{
			name:    "valid-key-with-version-0",
			key:     key,
			version: 0,
			wantErr: nil,
		},
		{
			name:    "invalid-key-size",
			key:     make([]byte, 16),
			version: 1,
			wantErr: ErrInvalidKeySize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := NewEncryptorWithVersion(tt.key, tt.version)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Nil(t, enc)
			} else {
				require.NoError(t, err)
				require.NotNil(t, enc)
				require.Equal(t, tt.version, enc.keyVersion)
			}
		})
	}
}

func TestEncryptedDataContainsVersion(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	testCases := []int{0, 1, 2, 99}

	for _, version := range testCases {
		t.Run("version-"+string(rune('0'+version)), func(t *testing.T) {
			enc, err := NewEncryptorWithVersion(key, version)
			require.NoError(t, err)

			encrypted, err := enc.Encrypt([]byte("secret"), nil)
			require.NoError(t, err)

			// Parse and verify version
			var encData EncryptedData
			err = json.Unmarshal(encrypted, &encData)
			require.NoError(t, err)
			require.Equal(t, version, encData.Version)
		})
	}
}

func TestGetEncryptedDataVersion(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	tests := []struct {
		name        string
		data        []byte
		wantVersion int
		wantErr     error
	}{
		{
			name: "valid-version-1",
			data: func() []byte {
				enc, _ := NewEncryptorWithVersion(key, 1)
				data, _ := enc.Encrypt([]byte("test"), nil)
				return data
			}(),
			wantVersion: 1,
		},
		{
			name: "valid-version-2",
			data: func() []byte {
				enc, _ := NewEncryptorWithVersion(key, 2)
				data, _ := enc.Encrypt([]byte("test"), nil)
				return data
			}(),
			wantVersion: 2,
		},
		{
			name: "unversioned-data-version-0",
			data: func() []byte {
				enc, _ := NewEncryptor(key) // Default encryptor has version 0
				data, _ := enc.Encrypt([]byte("test"), nil)
				return data
			}(),
			wantVersion: 0,
		},
		{
			name:    "empty-data",
			data:    []byte{},
			wantErr: ErrInvalidEncryptedData,
		},
		{
			name:    "invalid-json",
			data:    []byte("not json"),
			wantErr: ErrInvalidEncryptedData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := GetEncryptedDataVersion(tt.data)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantVersion, version)
			}
		})
	}
}

func TestVersionedEncryptionDecryption(t *testing.T) {
	// Generate two different keys
	key1, err := GenerateKey()
	require.NoError(t, err)
	key2, err := GenerateKey()
	require.NoError(t, err)

	// Create encryptors with different versions
	enc1, err := NewEncryptorWithVersion(key1, 1)
	require.NoError(t, err)
	enc2, err := NewEncryptorWithVersion(key2, 2)
	require.NoError(t, err)

	// Encrypt with version 1
	plaintext1 := []byte("secret encrypted with key v1")
	encrypted1, err := enc1.Encrypt(plaintext1, nil)
	require.NoError(t, err)

	// Encrypt with version 2
	plaintext2 := []byte("secret encrypted with key v2")
	encrypted2, err := enc2.Encrypt(plaintext2, nil)
	require.NoError(t, err)

	// Verify versions are stored correctly
	v1, err := GetEncryptedDataVersion(encrypted1)
	require.NoError(t, err)
	require.Equal(t, 1, v1)

	v2, err := GetEncryptedDataVersion(encrypted2)
	require.NoError(t, err)
	require.Equal(t, 2, v2)

	// Decrypt with correct keys
	decrypted1, err := enc1.Decrypt(encrypted1, nil)
	require.NoError(t, err)
	require.Equal(t, plaintext1, decrypted1)

	decrypted2, err := enc2.Decrypt(encrypted2, nil)
	require.NoError(t, err)
	require.Equal(t, plaintext2, decrypted2)

	// Cross-decryption should fail (wrong key)
	_, err = enc1.Decrypt(encrypted2, nil)
	require.ErrorIs(t, err, ErrDecryptionFailed)

	_, err = enc2.Decrypt(encrypted1, nil)
	require.ErrorIs(t, err, ErrDecryptionFailed)
}

func TestBackwardsCompatibilityWithUnversionedData(t *testing.T) {
	key, err := GenerateKey()
	require.NoError(t, err)

	// Create unversioned encryptor (simulates old code)
	unversionedEnc, err := NewEncryptor(key)
	require.NoError(t, err)

	// Create versioned encryptor with same key
	versionedEnc, err := NewEncryptorWithVersion(key, 0)
	require.NoError(t, err)

	plaintext := []byte("data from old system")

	// Encrypt with unversioned encryptor
	encrypted, err := unversionedEnc.Encrypt(plaintext, nil)
	require.NoError(t, err)

	// Should have version 0
	version, err := GetEncryptedDataVersion(encrypted)
	require.NoError(t, err)
	require.Equal(t, 0, version)

	// Should be decryptable by versioned encryptor with same key
	decrypted, err := versionedEnc.Decrypt(encrypted, nil)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}
