package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// ErrInvalidToken means a token must be rejected.
var ErrInvalidToken = errors.New("invalid token")

// tokenHeader reads a JWT's algorithm and key ID, before its signature is checked.
func tokenHeader(token string) (alg, kid string, err error) {
	head, _, ok := strings.Cut(token, ".")
	if !ok {
		return "", "", ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(head)
	if err != nil {
		return "", "", ErrInvalidToken
	}
	var h struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(raw, &h); err != nil {
		return "", "", ErrInvalidToken
	}
	return h.Alg, h.Kid, nil
}
