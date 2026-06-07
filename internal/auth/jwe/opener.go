/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package jwe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwe"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// Claims contains the parsed standard and custom claims from a verified token.
// Issuer and audience are validated during Open and not returned.
type Claims struct {
	Subject   string
	JTI       string
	IssuedAt  time.Time
	ExpiresAt time.Time
	Custom    map[string]any
}

// OpenerBuilder contains the data and logic needed to create a token opener. Don't create instances of this type
// directly, use the NewOpener function instead.
type OpenerBuilder struct {
	ctx               context.Context
	logger            *slog.Logger
	decryptionKeyFile string
	jwksURL           string
	issuer            string
	audience          string
	caPool            *x509.CertPool
}

// Opener decrypts nested JWTs (JWE, RSA-OAEP-256) and verifies the inner
// JWS (RS256) against a cached JWKS. Enforces single-use via JTI tracking.
type Opener struct {
	logger        *slog.Logger
	decryptionKey *privateKeyReloader
	jwksURL       string
	issuer        string
	audience      string
	jwksCache     *jwk.Cache
	mu            sync.Mutex
	seen          map[string]time.Time // jti -> cleanup expiry
}

// NewOpener creates a builder that can then be used to configure and create a new token opener.
func NewOpener() *OpenerBuilder {
	return &OpenerBuilder{}
}

// SetContext sets the context used for JWKS cache lifetime and the JTI cleanup goroutine. This is mandatory.
func (b *OpenerBuilder) SetContext(value context.Context) *OpenerBuilder {
	b.ctx = value
	return b
}

// SetLogger sets the logger. This is mandatory.
func (b *OpenerBuilder) SetLogger(value *slog.Logger) *OpenerBuilder {
	b.logger = value
	return b
}

// SetDecryptionKeyFile sets the path to the PEM private key for JWE decryption. This is mandatory.
func (b *OpenerBuilder) SetDecryptionKeyFile(value string) *OpenerBuilder {
	b.decryptionKeyFile = value
	return b
}

// SetJWKSURL sets the HTTPS URL of the JWKS endpoint for JWS verification. This is mandatory.
func (b *OpenerBuilder) SetJWKSURL(value string) *OpenerBuilder {
	b.jwksURL = value
	return b
}

// SetIssuer sets the expected JWT iss claim value. This is mandatory.
func (b *OpenerBuilder) SetIssuer(value string) *OpenerBuilder {
	b.issuer = value
	return b
}

// SetAudience sets the expected JWT aud claim value. This is mandatory.
func (b *OpenerBuilder) SetAudience(value string) *OpenerBuilder {
	b.audience = value
	return b
}

// SetCAPool sets the CA certificate pool for TLS when fetching JWKS. This is optional; if not set, system roots
// are used.
func (b *OpenerBuilder) SetCAPool(value *x509.CertPool) *OpenerBuilder {
	b.caPool = value
	return b
}

// Build uses the data stored in the builder to create and configure a new token opener.
func (b *OpenerBuilder) Build() (*Opener, error) {
	// Check parameters:
	if b.ctx == nil {
		return nil, errors.New("context is mandatory")
	}
	if b.logger == nil {
		return nil, errors.New("logger is mandatory")
	}
	if b.decryptionKeyFile == "" {
		return nil, errors.New("decryption key file is mandatory")
	}
	if b.jwksURL == "" {
		return nil, errors.New("JWKS URL is mandatory")
	}
	if b.issuer == "" {
		return nil, errors.New("issuer is mandatory")
	}
	if b.audience == "" {
		return nil, errors.New("audience is mandatory")
	}

	// Load the decryption key:
	decryptionKey := &privateKeyReloader{
		logger:  b.logger.With(slog.String("component", "token_opener_decryption")),
		keyFile: b.decryptionKeyFile,
	}
	if err := decryptionKey.ensureLoaded(b.ctx); err != nil {
		return nil, fmt.Errorf("initial load of decryption key: %w", err)
	}

	// OIDC Discovery (RFC 8414) requires jwks_uri to use HTTPS.
	parsed, err := url.Parse(b.jwksURL)
	if err != nil {
		return nil, fmt.Errorf("parse JWKS URL %q: %w", b.jwksURL, err)
	}
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("JWKS URL %q must use HTTPS scheme", b.jwksURL)
	}

	// Initialize the JWKS cache with automatic background refresh.
	cache, err := jwk.NewCache(b.ctx, httprc.NewClient())
	if err != nil {
		return nil, fmt.Errorf("create JWKS cache: %w", err)
	}
	var regOpts []jwk.RegisterOption
	if b.caPool != nil {
		regOpts = append(regOpts, jwk.WithHTTPClient(
			jwk.WrapHTTPClientDefaults(&http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						RootCAs:    b.caPool,
						MinVersion: tls.VersionTLS13,
					},
				},
			}),
		))
	}
	if err := cache.Register(b.ctx, b.jwksURL, regOpts...); err != nil {
		return nil, fmt.Errorf("register JWKS URL %q: %w", b.jwksURL, err)
	}

	// Create and populate the object:
	o := &Opener{
		logger:        b.logger,
		decryptionKey: decryptionKey,
		jwksURL:       b.jwksURL,
		issuer:        b.issuer,
		audience:      b.audience,
		jwksCache:     cache,
		seen:          make(map[string]time.Time),
	}
	go o.cleanupLoop(b.ctx)
	return o, nil
}

// Open decrypts the JWE outer layer, verifies the JWS inner layer against
// the cached JWKS, validates standard claims (iss, aud, exp with 5s skew),
// and enforces JTI single-use. Returns parsed claims on success.
func (o *Opener) Open(ctx context.Context, tokenString string) (*Claims, error) {
	if err := o.decryptionKey.ensureLoaded(ctx); err != nil {
		return nil, fmt.Errorf("reload decryption key: %w", err)
	}

	o.decryptionKey.mu.Lock()
	decPrivKey := o.decryptionKey.privateKey
	o.decryptionKey.mu.Unlock()

	if decPrivKey == nil {
		return nil, errors.New("decryption private key not loaded")
	}

	// Step 1: Decrypt JWE outer layer.
	jwsPayload, err := jwe.Decrypt([]byte(tokenString),
		jwe.WithKey(jwa.RSA_OAEP_256(), decPrivKey),
	)
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}

	// Step 2: Look up cached JWKS and verify JWS inner layer.
	set, err := o.jwksCache.Lookup(ctx, o.jwksURL)
	if err != nil {
		o.logger.WarnContext(ctx, "JWKS cache lookup failed",
			slog.String("jwks_url", o.jwksURL),
			slog.Any("error", err),
		)
		return nil, fmt.Errorf("lookup JWKS from cache: %w", err)
	}

	parseOpts := []jwt.ParseOption{
		jwt.WithKeySet(set),
		jwt.WithIssuer(o.issuer),
		jwt.WithAcceptableSkew(5 * time.Second),
	}
	parseOpts = append(parseOpts, jwt.WithAudience(o.audience))

	tok, err := jwt.Parse(jwsPayload, parseOpts...)
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}

	// Step 3: Extract standard claims.
	jti, ok := tok.JwtID()
	if !ok || jti == "" {
		return nil, errors.New("token missing jti")
	}

	sub, ok := tok.Subject()
	if !ok || sub == "" {
		return nil, errors.New("token missing sub")
	}

	exp, ok := tok.Expiration()
	if !ok {
		return nil, errors.New("token missing exp")
	}

	iat, _ := tok.IssuedAt()

	// Step 4: Extract custom claims (everything not in the standard set).
	custom := make(map[string]any)
	for _, key := range tok.Keys() {
		switch key {
		case "iss", "sub", "aud", "jti", "iat", "exp", "nbf":
			// standard claims handled above
		default:
			var val any
			if err := tok.Get(key, &val); err == nil {
				custom[key] = val
			}
		}
	}

	// Step 5: JTI single-use check.
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, exists := o.seen[jti]; exists {
		return nil, fmt.Errorf("token already used (jti: %s)", jti)
	}
	o.seen[jti] = exp.Add(5 * time.Second)

	return &Claims{
		Subject:   sub,
		JTI:       jti,
		IssuedAt:  iat,
		ExpiresAt: exp,
		Custom:    custom,
	}, nil
}

// cleanupLoop periodically removes expired JTI entries. It stops when ctx
// is cancelled (graceful shutdown).
func (o *Opener) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.mu.Lock()
			now := time.Now()
			for jti, expiry := range o.seen {
				if now.After(expiry) {
					delete(o.seen, jti)
				}
			}
			o.mu.Unlock()
		}
	}
}
