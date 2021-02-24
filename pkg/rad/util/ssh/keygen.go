// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// DefaultSize is the recommended default size for an SSH key.
const DefaultSize = 4096

// GenerateKey generates an SSH key in memory and returns the public key in the .pub format.
func GenerateKey(size int) ([]byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, size)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to generate ssh key: %v", err)
	}

	err = privateKey.Validate()
	if err != nil {
		return []byte{}, fmt.Errorf("failed to generate ssh key: %v", err)
	}

	publicRsaKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to generate ssh key: %v", err)
	}

	return ssh.MarshalAuthorizedKey(publicRsaKey), nil
}
