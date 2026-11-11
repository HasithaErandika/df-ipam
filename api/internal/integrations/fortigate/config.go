package fortigate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/secrets"
)

// The token is encrypted with the master key from api/.env; the database holds
// only ciphertext, tied to this setting by the associated data.
var tokenAAD = []byte("settings:fortigate.token")

// ErrNoKey means SECRET_ENCRYPTION_KEY is missing, so a token can't be stored.
var ErrNoKey = errors.New("set SECRET_ENCRYPTION_KEY in api/.env before saving a FortiGate token")

// Config is the connection an Admin set up in Settings.
type Config struct {
	Enabled         bool   `json:"enabled"`
	URL             string `json:"url"`
	VDOM            string `json:"vdom"`
	VerifyTLS       bool   `json:"verifyTls"`
	IntervalMinutes int    `json:"intervalMinutes"`
	HasToken        bool   `json:"hasToken"`
	token           string
}

type sealed struct {
	Data    string `json:"data"`
	Version int    `json:"version"`
}

// Load reads the settings and decrypts the token when there is one.
func Load(ctx context.Context, conn db.Conn, keys *secrets.Keyring) (Config, error) {
	cfg := Config{VerifyTLS: true, IntervalMinutes: 10}
	rows, err := conn.Query(ctx, `SELECT key, value FROM settings WHERE key LIKE 'fortigate.%'`)
	if err != nil {
		return cfg, err
	}
	defer rows.Close()
	var tok sealed
	for rows.Next() {
		var k string
		var v json.RawMessage
		if err := rows.Scan(&k, &v); err != nil {
			return cfg, err
		}
		switch k {
		case "fortigate.enabled":
			_ = json.Unmarshal(v, &cfg.Enabled)
		case "fortigate.url":
			_ = json.Unmarshal(v, &cfg.URL)
		case "fortigate.vdom":
			_ = json.Unmarshal(v, &cfg.VDOM)
		case "fortigate.verify_tls":
			_ = json.Unmarshal(v, &cfg.VerifyTLS)
		case "fortigate.interval_minutes":
			_ = json.Unmarshal(v, &cfg.IntervalMinutes)
		case "fortigate.token":
			_ = json.Unmarshal(v, &tok)
		}
	}
	if err := rows.Err(); err != nil {
		return cfg, err
	}
	cfg.HasToken = tok.Data != ""
	if cfg.HasToken && keys != nil {
		raw, err := base64.StdEncoding.DecodeString(tok.Data)
		if err == nil {
			plain, derr := keys.Decrypt(raw, tok.Version, tokenAAD)
			if derr != nil {
				return cfg, errors.New("the stored FortiGate token can't be decrypted: save it again")
			}
			cfg.token = string(plain)
		}
	}
	return cfg, nil
}

// SealToken encrypts a token for storage as a setting value.
func SealToken(keys *secrets.Keyring, token string) (string, error) {
	if keys == nil {
		return "", ErrNoKey
	}
	data, version, err := keys.Encrypt([]byte(token), tokenAAD)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(sealed{Data: base64.StdEncoding.EncodeToString(data), Version: version})
	return string(out), err
}

// Client builds a client from the saved settings.
func (c Config) Client() (*Client, error) {
	if c.URL == "" || c.token == "" {
		return nil, errors.New("enter the FortiGate address and an API token first")
	}
	return NewClient(c.URL, c.token, c.VDOM, c.VerifyTLS)
}
