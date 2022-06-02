package authentication

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type publicKey struct {
	kty string `json:"kty"`
	use string `json:"use"`
	kid string `json:"kid"`
	x5t string `json:"x5t"`
	n   string `json:"n"`
	e   string `json:"e"`
	x5c string `json;"x5c"`
}

const AADPublicKeyURL = "https://login.microsoftonline.com/common/discovery/keys"

// fetchAADPublicKey fetches the common public keys from the AADPublicKeyURL
func fetchAADPublicKey() ([]publicKey, error) {
	client := http.Client{}
	resp, err := client.Get(AADPublicKeyURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, err
	} else if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("Response code - %d", resp.StatusCode)
		return nil, errors.New(msg)
	}
	defer resp.Body.Close()
	var keys []publicKey
	err = json.NewDecoder(resp.Body).Decode(&keys)
	if err != nil {
		return nil, err
	}
	return keys, nil
}
