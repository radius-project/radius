package authentication

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/radlogger"
	"gopkg.in/square/go-jose.v2/jwt"
)

var (
	ErrTokenHeader = errors.New("Invalid Authorization Header")
	ErrToken       = errors.New("Invalid Token")
)

const (
	PoPTokenAlg      = "RS256"
	PoPTokenValidity = 15
)

// ValidateAuthHeader fetches the authentication token in the request header
func ValidateAuthHeader(req *http.Request, log logr.Logger) (string, bool, error) {
	fmt.Println("retrieve token from header")
	auth := req.Header.Get("Authorization")
	if auth == "" {
		log.V(radlogger.Error).Info(ErrTokenHeader.Error(), "- Authorization header is missing")
		return "", false, ErrTokenHeader
	}
	log.Info("Authorization header token retrieved is - ", auth)
	parts := strings.Split(auth, " ")
	if len(parts) < 2 || strings.ToLower(parts[0]) != "bearer" {
		log.V(radlogger.Error).Info(ErrTokenHeader.Error(), "- Bearer missing")
		return "", true, ErrTokenHeader
	}
	token := parts[1]
	log.Info("POP token retrieved is - ", token)
	return token, true, nil
}

// Validate checks the validity of the token by verifying the signature and the access token present in the POP token
func Validate(token string, identityOptions hostoptions.IdentityOptions, log logr.Logger) error {
	//POP token should have 3 parts separated by '.' . The second part contains the JWT claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		log.V(radlogger.Error).Info("Invalid token Length - ", len(parts))
		return ErrToken
	}
	isSignatureValid, err := validateSignature(parts)
	if !isSignatureValid {
		log.V(radlogger.Error).Info("Token signed with invalid signature - ", len(parts))
		return ErrToken
	}
	tmp, err := jwt.ParseSigned(token)
	if err != nil {
		log.V(radlogger.Error).Info("PoP token parsing failed with error - ", err.Error())
		return ErrToken
	}
	if strings.ToUpper(tmp.Headers[0].Algorithm) != PoPTokenAlg {
		log.V(radlogger.Error).Info("Incorrect Algorithm in the POP token header")
		return ErrToken
	}
	if typ, ok := tmp.Headers[0].ExtraHeaders["typ"].(string); !ok || strings.ToLower(typ) != "pop" {
		log.V(radlogger.Error).Info("Incorrect typ in the PoP token header")
		return ErrToken
	}
	var tokenClaims map[string]interface{}
	err = tmp.UnsafeClaimsWithoutVerification(&tokenClaims)
	if err != nil {
		log.V(radlogger.Error).Info("Cannot decode PoP token - ", err.Error())
		return ErrToken
	}
	// validate the token expiry.
	isExpired, err := validateExpiry(tokenClaims)
	if isExpired {
		log.V(radlogger.Error).Info("PoP token expired")
		return ErrToken
	} else if err != nil {
		log.V(radlogger.Error).Info(err.Error())
		return ErrToken
	}
	//validate hostname in the token
	if uClaim, ok := tokenClaims["u"]; ok {
		if u, ok := uClaim.(string); !ok {
			return errors.New("Invalid claim. u claim should be string")
		} else if u != identityOptions.HostName {
			return errors.New("Invalid hostname")
		}

	} else {
		return errors.New("Invlaid PoP token. access token missing")
	}
	// validate the access token
	err = validateAccessToken(tokenClaims, identityOptions)
	if err != nil {
		log.V(radlogger.Error).Info(err.Error())
		return ErrToken
	}
	return nil
}

// validateSignature validates if the token is signed by AAD public key
func validateSignature(parts []string) (bool, error) {
	keys, err := fetchAADPublicKey()
	message := fmt.Sprintf("%s.%s", parts[0], parts[1])
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false, fmt.Errorf("Failed to decode signed message - %+v", err)
	}
	isSigned := true
	for _, key := range keys {
		n, _ := base64.RawURLEncoding.DecodeString(key.n)
		e, _ := base64.RawURLEncoding.DecodeString(key.e)
		z := new(big.Int)
		z.SetBytes(n)
		var buffer bytes.Buffer
		buffer.WriteByte(0)
		buffer.Write([]byte(e))
		exponent := binary.BigEndian.Uint32(buffer.Bytes())
		publicKey := &rsa.PublicKey{N: z, E: int(exponent)}

		hasher := crypto.SHA256.New()
		_, err = hasher.Write([]byte(message))
		if err != nil {
			return false, fmt.Errorf("Failed to write message to hasher - %+v", err)
		}
		err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hasher.Sum(nil), signature)
		if err != nil {
			isSigned = false
			break
		}
	}
	if !isSigned {
		return isSigned, fmt.Errorf("PoP token signature verification failed - %+v", err)
	}
	return isSigned, nil
}

// validateAccessToken verifies the access token claims to validate the audience and clientId
func validateAccessToken(tokenClaims map[string]interface{}, identityOptions hostoptions.IdentityOptions) error {
	var at string
	if atClaim, ok := tokenClaims["at"]; ok {
		if at, ok = atClaim.(string); !ok {
			return errors.New("Invalid access token. at claim should be string")
		}
	} else {
		return errors.New("Invlaid PoP token. access token missing")
	}
	accessToken, err := jwt.ParseSigned(at)
	if err != nil {
		return fmt.Errorf("Access token parsing failed with error - ", err.Error())
	}
	var atClaims map[string]interface{}
	err = accessToken.UnsafeClaimsWithoutVerification(&atClaims)
	if err != nil {
		return fmt.Errorf("Cannot decode access token - ", err.Error())
	}
	if atClaims["appid"].(string) != identityOptions.ClientID {
		return errors.New("Invalid clientId in the access token")
	}
	if atClaims["aud"].(string) != identityOptions.ArmEndpoint {
		return errors.New("Invalid audience in the access token")
	}
	return nil
}

// validateExpiry verifies the POP token expiry. The default expiry is 15 min.
func validateExpiry(tokenClaims map[string]interface{}) (bool, error) {
	now := time.Now()
	var issuedTime time.Time
	if ts, ok := tokenClaims["ts"]; ok {
		switch iat := ts.(type) {
		case float64:
			issuedTime = time.Unix(int64(iat), 0)
		case int64:
			issuedTime = time.Unix(iat, 0)
		case string:
			v, _ := strconv.ParseInt(iat, 10, 64)
			issuedTime = time.Unix(v, 0)
		}
		expireat := issuedTime.Add(PoPTokenValidity * time.Minute)
		if expireat.Before(now) {
			return false, nil
		}
	} else {
		return false, errors.New("Invalid token. Claim missing field ts")
	}
	return true, nil
}
