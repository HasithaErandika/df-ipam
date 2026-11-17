package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
)

const (
	jwksTTL        = 10 * time.Minute // Microsoft advises against caching longer
	jwksMinRefetch = 30 * time.Second // limits refetches caused by unknown key IDs
)

// keyCache holds a provider's signing keys. An unknown key ID refetches at most
// every 30 seconds, and a brief outage is ridden out with the keys already held.
type keyCache struct {
	url string

	mu        sync.Mutex
	keys      jwk.Set
	fetchedAt time.Time
}

func (k *keyCache) get(ctx context.Context, kid string) (jwk.Set, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	age := time.Since(k.fetchedAt)
	stale := k.keys == nil || age > jwksTTL
	if !stale {
		if _, known := k.keys.LookupKeyID(kid); !known && age > jwksMinRefetch {
			stale = true
		}
	}
	if stale {
		fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		keys, err := jwk.Fetch(fetchCtx, k.url)
		if err != nil {
			if k.keys != nil && age < 2*jwksTTL {
				return k.keys, nil
			}
			return nil, fmt.Errorf("fetch jwks: %w", err)
		}
		pinRS256(keys)
		k.keys, k.fetchedAt = keys, time.Now()
	}
	return k.keys, nil
}

// pinRS256 labels Microsoft's RSA signing keys RS256. Microsoft publishes them
// without "alg", and the JWT library uses no key without one. Pinning, not
// inferring, means a token signed any other way still finds no key.
func pinRS256(keys jwk.Set) {
	for i := range keys.Len() {
		key, ok := keys.Key(i)
		if !ok || key.KeyType() != jwa.RSA() {
			continue
		}
		if _, has := key.Algorithm(); !has {
			_ = key.Set(jwk.AlgorithmKey, jwa.RS256())
		}
	}
}
