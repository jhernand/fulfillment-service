/*
Copyright (c) 2025 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package auth

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"

	kubefiles "github.com/osac-project/fulfillment-service/internal/kubernetes/files"
)

// GrpcAuthnInterceptorBuilder contains the data and logic needed to build an interceptor that validates JWT tokens
// using JSON Web Key Sets. Don't create instances of this type directly, use the NewGrpcAuthnInterceptor function
// instead.
type GrpcAuthnInterceptorBuilder struct {
	logger           *slog.Logger
	rootDir          string
	issuerUrls       []string
	kubeIssuer       bool
	anonymousMethods []string
	caPool           *x509.CertPool
	tolerance        time.Duration
	keysTTL          time.Duration
}

// GrpcAuthnInterceptor is a gRPC interceptor that validates JWT tokens using one or more JSON Web Key Sets. It extracts
// the bearer token from the authorization header, validates it against the configured JWKS endpoints, and stores the
// validated token in the context for downstream interceptors and handlers. The JWKS endpoints are discovered
// automatically via OpenID Connect discovery for each trusted issuer.
type GrpcAuthnInterceptor struct {
	logger           *slog.Logger
	rootDir          string
	anonymousMethods []*regexp.Regexp
	issuersInfo      map[string]*grpcAuthnIssuerInfo
	kubeIssuer       bool
	tokenParser      *jwt.Parser
	httpClient       *http.Client
	keysCache        sync.Map
	keysCacheLock    sync.Mutex
	lastRefresh      time.Time
	keysTTL          time.Duration
}

// grpcAuthnKeyCacheTag is a tag that identifies a key in the keys cache.
type grpcAuthnKeyCacheTag struct {
	issuerUrl string
	keyId     string
}

// grpcAuthnKeyCacheEntry is a value in the keys cache, wrapping the actual key with metadata.
type grpcAuthnKeyCacheEntry struct {
	issuerUrl string
	keyAny    any
	loadedAt  time.Time
}

// grpcAuthnIssuerInfo holds discovered information about a trusted issuer.
type grpcAuthnIssuerInfo struct {
	// issuerUrl is the URL of the issuer.
	issuerUrl string

	// jwksUrl is the URL of the JSON web key set, discovered via OpenID Connect discovery. It is empty until
	// discovery is performed.
	jwksUrl string

	// tokenFile is the file that contains the bearer token that will be used to authenticate to the OpenID
	// discovery and JSON web key set endpoints for this issuer.
	tokenFile string
}

// NewGrpcAuthnInterceptor creates a builder that can then be used to configure and create a new JWKS authentication
// interceptor.
func NewGrpcAuthnInterceptor() *GrpcAuthnInterceptorBuilder {
	return &GrpcAuthnInterceptorBuilder{
		keysTTL: 1 * time.Hour,
	}
}

// SetLogger sets the logger that will be used to write to the log. This is mandatory.
func (b *GrpcAuthnInterceptorBuilder) SetLogger(value *slog.Logger) *GrpcAuthnInterceptorBuilder {
	b.logger = value
	return b
}

// AddAnonymousMethodRegex adds a regular expression that describes a set of methods that are considered anonymous, and
// therefore require no authentication. The regular expression will be matched against the full gRPC method name,
// including the leading slash. For example, to consider anonymous all the methods of the 'example.v1.Products' service
// the regular expression could be '^/example\.v1\.Products/.*$'.
//
// This method may be called multiple times to add multiple regular expressions. A method will be considered anonymous
// if it matches at least one of them.
func (b *GrpcAuthnInterceptorBuilder) AddAnonymousMethodRegex(value string) *GrpcAuthnInterceptorBuilder {
	b.anonymousMethods = append(b.anonymousMethods, value)
	return b
}

// AddIssuer adds a trusted token issuer URL. At least one token issuer must be configured, either using this method,
// the AddIssuers method or the AddKubernetesIssuer method.
func (b *GrpcAuthnInterceptorBuilder) AddIssuer(value string) *GrpcAuthnInterceptorBuilder {
	b.issuerUrls = append(b.issuerUrls, value)
	return b
}

// AddIssuers adds a list of trusted token issuer URLs. At least one token issuer must be configured, either using this
// method, the AddIssuer method or the AddKubernetesIssuer method.
func (b *GrpcAuthnInterceptorBuilder) AddIssuers(values ...string) *GrpcAuthnInterceptorBuilder {
	b.issuerUrls = append(b.issuerUrls, values...)
	return b
}

// AddKubernetesIssuer specifies if the Kubernetes API server issuer should be automatically added to the list of
// trusted issuers. When this is set to 'true', the interceptor will check if it is running inside a Kubernetes pod, and
// if so, it will automatically add the Kubernetes issuer URL to the list of trusted issuers. The default is 'false'.
func (b *GrpcAuthnInterceptorBuilder) AddKubernetesIssuer(value bool) *GrpcAuthnInterceptorBuilder {
	b.kubeIssuer = value
	return b
}

// SetCaPool sets the certificate authorities that will be trusted when verifying the TLS certificate of the servers
// where JWKS are loaded from.
func (b *GrpcAuthnInterceptorBuilder) SetCaPool(value *x509.CertPool) *GrpcAuthnInterceptorBuilder {
	b.caPool = value
	return b
}

// SetTolerance sets the maximum time that a token will be considered valid after it has expired.
func (b *GrpcAuthnInterceptorBuilder) SetTolerance(value time.Duration) *GrpcAuthnInterceptorBuilder {
	b.tolerance = value
	return b
}

// SetKeysTTL sets the time-to-live for cached JSON web keys. When a request finds a cached key older than this
// duration, it uses the key immediately but triggers an asynchronous refresh in the background. The default is one
// hour. Setting it to zero disables TTL-based refresh, so cached keys are only reloaded on a cache miss.
func (b *GrpcAuthnInterceptorBuilder) SetKeysTTL(value time.Duration) *GrpcAuthnInterceptorBuilder {
	b.keysTTL = value
	return b
}

// SetRoot sets a custom root directory for resolving file paths. This method is primarily intended for unit tests where
// you need to simulate the presence of files (like Kubernetes token files) in a controlled environment. When a root is
// set, all file paths (both absolute and relative) will be resolved relative to this root directory. This affects the
// check of whether the process is running inside a Kubernetes pod.
//
// For regular use, there is typically no need to call this method as the default behavior of using paths as-is from the
// filesystem is appropriate for production environments.
func (b *GrpcAuthnInterceptorBuilder) SetRoot(value string) *GrpcAuthnInterceptorBuilder {
	b.rootDir = value
	return b
}

// Build uses the data stored in the builder to create and configure a new interceptor.
func (b *GrpcAuthnInterceptorBuilder) Build() (result *GrpcAuthnInterceptor, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.tolerance < 0 {
		err = errors.New("tolerance must be zero or positive")
		return
	}
	if b.keysTTL < 0 {
		err = errors.New("keys TTL must be zero or positive")
		return
	}
	if b.caPool == nil {
		err = errors.New("CA pool is mandatory")
		return
	}

	// Create the JWT parser:
	parserOptions := []jwt.ParserOption{
		jwt.WithValidMethods([]string{
			"RS256", "RS384", "RS512",
		}),
		jwt.WithIssuedAt(),
		jwt.WithExpirationRequired(),
	}
	if b.tolerance > 0 {
		parserOptions = append(parserOptions, jwt.WithLeeway(b.tolerance))
	}
	tokenParser := jwt.NewParser(parserOptions...)

	// Check the list of issuers provided by the caller:
	issuersInfo := map[string]*grpcAuthnIssuerInfo{}
	for _, issuerUrl := range b.issuerUrls {
		issuerUrl = strings.TrimSpace(issuerUrl)
		if issuerUrl == "" {
			err = errors.New("invalid empty issuer URL")
			return
		}
		var parsed *url.URL
		parsed, err = url.Parse(issuerUrl)
		if err != nil {
			err = fmt.Errorf("invalid issuer URL '%s': %w", issuerUrl, err)
			return
		}
		if parsed.Scheme != "https" {
			err = fmt.Errorf("issuer URL '%s' must use the HTTPS scheme", issuerUrl)
			return
		}
		issuerUrl = parsed.String()
		issuersInfo[issuerUrl] = &grpcAuthnIssuerInfo{
			issuerUrl: issuerUrl,
		}
	}

	// Add the Kubernetes issuer, if requested by the caller:
	if b.kubeIssuer {
		kubeTokenFile := b.resolvePath(kubefiles.ServiceAccountToken)
		var kubeTokenObject *jwt.Token
		kubeTokenObject, err = b.loadKubeToken(kubeTokenFile, tokenParser)
		if err != nil {
			return
		}
		if kubeTokenObject != nil {
			var kubeIssuerUrl string
			kubeIssuerUrl, err = kubeTokenObject.Claims.GetIssuer()
			if err != nil {
				err = fmt.Errorf(
					"failed to get the 'iss' claim from Kubernetes service account token: %w",
					err,
				)
				return
			}
			issuersInfo[kubeIssuerUrl] = &grpcAuthnIssuerInfo{
				issuerUrl: kubeIssuerUrl,
				tokenFile: kubeTokenFile,
			}
		}
	}

	// Check that at least one issuer has been configured, as otherwise the interceptor will not be able to
	// authenticate any request.
	if len(issuersInfo) == 0 {
		err = errors.New("at least one issuer must be configured")
		return
	}

	// Compile public method regexes:
	anonymousMethods := make([]*regexp.Regexp, len(b.anonymousMethods))
	for i, expr := range b.anonymousMethods {
		anonymousMethods[i], err = regexp.Compile(expr)
		if err != nil {
			err = fmt.Errorf("failed to compile anonymous method regex '%s': %w", expr, err)
			return
		}
	}

	// Create the HTTP client used to download JSON web key sets:
	httpClient := &http.Client{
		Transport: &http.Transport{
			ResponseHeaderTimeout: 10 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			TLSClientConfig: &tls.Config{
				RootCAs: b.caPool,
			},
		},
	}

	// Create and populate the object:
	result = &GrpcAuthnInterceptor{
		logger:           b.logger,
		rootDir:          b.rootDir,
		issuersInfo:      issuersInfo,
		kubeIssuer:       b.kubeIssuer,
		anonymousMethods: anonymousMethods,
		tokenParser:      tokenParser,
		httpClient:       httpClient,
		keysTTL:          b.keysTTL,
	}
	return
}

// loadKubeToken loads the Kubernetes service account token file and parses it using the provided token parser, but
// without verifying the signature. Returns nil if the file does not exist.
func (b *GrpcAuthnInterceptorBuilder) loadKubeToken(tokenFile string, tokenParser *jwt.Parser) (result *jwt.Token,
	err error) {
	// Try to load the file:
	tokenBytes, err := os.ReadFile(tokenFile) //nolint:gosec
	if errors.Is(err, os.ErrNotExist) {
		err = nil
		return
	}
	if err != nil {
		err = fmt.Errorf(
			"failed to read Kubernetes service account token file '%s': %w",
			tokenFile, err,
		)
		return
	}
	tokenText := strings.TrimSpace(string(tokenBytes))

	// Try to parset the token, but without verifying the signature:
	tokenObject, _, err := tokenParser.ParseUnverified(tokenText, jwt.MapClaims{})
	if err != nil {
		err = fmt.Errorf(
			"failed to parse Kubernetes service account token from file '%s': %w",
			tokenFile, err,
		)
		return
	}

	// Return the token:
	result = tokenObject
	return
}

// resolvePath resolves a file path using the custom root directory if set. If no root is set, returns the original
func (b *GrpcAuthnInterceptorBuilder) resolvePath(path string) string {
	if b.rootDir == "" {
		return path
	}
	if filepath.IsAbs(path) {
		return filepath.Join(b.rootDir, path[1:])
	}
	return filepath.Join(b.rootDir, path)
}

// UnaryServer is the unary server interceptor function.
func (i *GrpcAuthnInterceptor) UnaryServer(ctx context.Context, request any,
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response any, err error) {
	ctx, err = i.authenticate(ctx, info.FullMethod)
	if err != nil {
		return
	}
	return handler(ctx, request)
}

// StreamServer is the stream server interceptor function.
func (i *GrpcAuthnInterceptor) StreamServer(server any, stream grpc.ServerStream,
	info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx, err := i.authenticate(stream.Context(), info.FullMethod)
	if err != nil {
		return err
	}
	stream = &grpcAuthnStream{
		context: ctx,
		stream:  stream,
	}
	return handler(server, stream)
}

// grpcAuthnStream wraps a gRPC server stream with a modified context.
type grpcAuthnStream struct {
	context context.Context
	stream  grpc.ServerStream
}

func (s *grpcAuthnStream) Context() context.Context {
	return s.context
}

func (s *grpcAuthnStream) RecvMsg(message any) error {
	return s.stream.RecvMsg(message)
}

func (s *grpcAuthnStream) SendHeader(md metadata.MD) error {
	return s.stream.SendHeader(md)
}

func (s *grpcAuthnStream) SendMsg(message any) error {
	return s.stream.SendMsg(message)
}

func (s *grpcAuthnStream) SetHeader(md metadata.MD) error {
	return s.stream.SetHeader(md)
}

func (s *grpcAuthnStream) SetTrailer(md metadata.MD) {
	s.stream.SetTrailer(md)
}

// authenticate extracts and validates the bearer token from the gRPC metadata, then stores the parsed token in the
// context.
func (i *GrpcAuthnInterceptor) authenticate(ctx context.Context, method string) (result context.Context, err error) {
	values := metadata.ValueFromIncomingContext(ctx, Authorization)
	length := len(values)
	switch length {
	case 0:
		result, err = i.authenticateWithoutToken(ctx, method)
	case 1:
		result, err = i.authenticateWithToken(ctx, method, values[0])
	default:
		err = grpcstatus.Errorf(
			grpccodes.Unauthenticated,
			"expected at most one 'authorization' header, but received %d",
			length,
		)
	}
	return
}

func (i *GrpcAuthnInterceptor) authenticateWithToken(ctx context.Context, method string,
	auth string) (result context.Context, err error) {
	token, err := i.checkAuth(ctx, auth)
	if err != nil {
		if i.isAnonymousMethod(method) {
			result = ContextWithToken(ctx, nil)
			err = nil
		}
		return
	}
	result = ContextWithToken(ctx, token)
	return
}

func (i *GrpcAuthnInterceptor) authenticateWithoutToken(ctx context.Context, method string) (result context.Context,
	err error) {
	isAnonymous := i.isAnonymousMethod(method)
	haveIssuers := len(i.issuersInfo) > 0
	if isAnonymous || !haveIssuers {
		result = ContextWithToken(ctx, nil)
		return
	}
	err = grpcstatus.Errorf(grpccodes.Unauthenticated, "method '%s' requires authentication", method)
	return
}

// checkAuth extracts the bearer token from the authorization header and validates it.
func (i *GrpcAuthnInterceptor) checkAuth(ctx context.Context, auth string) (result *jwt.Token, err error) {
	bearer, err := i.extractBearer(auth)
	if err != nil {
		return
	}
	result, err = i.checkToken(ctx, bearer)
	if err != nil {
		return
	}
	claims, ok := result.Claims.(jwt.MapClaims)
	if !ok {
		err = grpcstatus.Error(grpccodes.Unauthenticated, "bearer token claims are invalid")
		return
	}
	err = i.checkClaims(claims)
	if err != nil {
		return
	}
	return
}

// extractBearer extracts the bearer token from the authorization header value.
func (i *GrpcAuthnInterceptor) extractBearer(auth string) (result string, err error) {
	matches := authnBearerRE.FindStringSubmatch(auth)
	if len(matches) != 3 {
		err = grpcstatus.Error(
			grpccodes.Unauthenticated,
			"authorization header value should be 'Bearer TOKEN'",
		)
		return
	}
	scheme := matches[1]
	if !strings.EqualFold(scheme, "Bearer") {
		err = grpcstatus.Errorf(
			grpccodes.Unauthenticated,
			"authentication scheme '%s' is not supported",
			scheme,
		)
		return
	}
	result = matches[2]
	return
}

// checkToken validates the token signature and standard claims.
func (i *GrpcAuthnInterceptor) checkToken(ctx context.Context, bearer string) (result *jwt.Token, err error) {
	// Parse the token and verify the signature, and return if it succeeds:
	result, err = i.tokenParser.ParseWithClaims(
		bearer, jwt.MapClaims{},
		func(token *jwt.Token) (key any, err error) {
			return i.selectKey(ctx, token)
		},
	)
	if err == nil {
		return
	}

	// The error returned by the token parser may be a gRPC status error, but wrapped by the parsere with it's own
	// error messages. We don't want to send that to the client, so we need to unwrap it, and to do so we need to
	// use a type that matches the error interface used internally by the gRPC package.
	type grpcError interface {
		error
		GRPCStatus() *grpcstatus.Status
	}
	grpcErr, ok := errors.AsType[grpcError](err)
	if ok {
		err = grpcErr.GRPCStatus().Err()
		return
	}

	// For errors specific to the JWT library, translate them to the appropriate gRPC status error.
	switch {
	case errors.Is(err, jwt.ErrTokenMalformed):
		err = grpcstatus.Error(grpccodes.Unauthenticated, "bearer token is malformed")
	case errors.Is(err, jwt.ErrTokenUnverifiable):
		err = grpcstatus.Error(grpccodes.Unauthenticated, "bearer token cannot be verified")
	case errors.Is(err, jwt.ErrSignatureInvalid):
		err = grpcstatus.Error(grpccodes.Unauthenticated, "signature of bearer token is not valid")
	case errors.Is(err, jwt.ErrTokenExpired):
		err = grpcstatus.Error(grpccodes.Unauthenticated, "bearer token is expired")
	case errors.Is(err, jwt.ErrTokenUsedBeforeIssued):
		err = grpcstatus.Error(grpccodes.Unauthenticated, "bearer token was issued in the future")
	case errors.Is(err, jwt.ErrTokenNotValidYet):
		err = grpcstatus.Error(grpccodes.Unauthenticated, "bearer token is not valid yet")
	default:
		err = grpcstatus.Error(grpccodes.Unauthenticated, "bearer token is not valid")
	}

	return
}

// checkClaims checks that the required claims are present and have valid values.
func (i *GrpcAuthnInterceptor) checkClaims(claims jwt.MapClaims) error {
	value, ok := claims["typ"]
	if ok {
		typ, ok := value.(string)
		if !ok {
			return grpcstatus.Errorf(
				grpccodes.Unauthenticated,
				"bearer token type claim contains incorrect value '%v'",
				value,
			)
		}
		if !strings.EqualFold(typ, "Bearer") {
			return grpcstatus.Errorf(
				grpccodes.Unauthenticated,
				"bearer token type '%s' is not allowed",
				typ,
			)
		}
	}
	_, ok = claims["sub"].(string)
	if !ok {
		return grpcstatus.Error(grpccodes.Unauthenticated, "bearer token does not contain required claim 'sub'")
	}

	return nil
}

// selectKey selects the key that should be used to verify the signature of the given token.
func (i *GrpcAuthnInterceptor) selectKey(ctx context.Context, token *jwt.Token) (result any, err error) {
	// Check the issuer:
	issuerUrl, err := token.Claims.GetIssuer()
	if err != nil {
		err = fmt.Errorf("failed to get token issuer: %w", err)
		return
	}
	issuerInfo, ok := i.issuersInfo[issuerUrl]
	if !ok {
		err = grpcstatus.Errorf(grpccodes.Unauthenticated, "token issuer '%s' is not trusted", issuerUrl)
		return
	}

	// Build the cache key:
	keyId, ok := token.Header["kid"].(string)
	if !ok || keyId == "" {
		err = fmt.Errorf("token does not contain the 'kid' field in the header")
		return
	}
	keyTag := grpcAuthnKeyCacheTag{
		issuerUrl: issuerUrl,
		keyId:     keyId,
	}

	// Try to find the key in the cache. If found but expired according to the TTL, return it immediately (so the
	// current request is not delayed) but trigger a background refresh.
	value, ok := i.keysCache.Load(keyTag)
	if ok {
		entry := value.(grpcAuthnKeyCacheEntry)
		if i.keysTTL > 0 && time.Since(entry.loadedAt) > i.keysTTL {
			go func() {
				err := i.refreshKeys(ctx, issuerInfo)
				if err != nil {
					i.logger.ErrorContext(
						ctx,
						"Failed to refresh JSON web keys",
						slog.Any("error", err),
					)
				}
			}()
		}
		result = entry.keyAny
		return
	}

	// If the key is not in the cache, try to refresh synchronously:
	err = i.refreshKeys(ctx, issuerInfo)
	if err != nil {
		return
	}

	// Try again after refresh:
	value, ok = i.keysCache.Load(keyTag)
	if ok {
		entry := value.(grpcAuthnKeyCacheEntry)
		result = entry.keyAny
		return
	}

	// Report that we do not have a matching key:
	err = fmt.Errorf("no key found with issuer '%s' and key identifier '%s'", issuerUrl, keyId)
	return
}

// refreshKeys loads all JSON web key sets into a temporary map and, only if that succeeds, replaces the live cache.
// This avoids leaving the cache empty when a network condition prevents reaching the remote key sets. The method is
// throttled to at most once per minute.
func (i *GrpcAuthnInterceptor) refreshKeys(ctx context.Context, issuerInfo *grpcAuthnIssuerInfo) error {
	// Return immediately if the last refresh was less than a minute ago:
	i.keysCacheLock.Lock()
	defer i.keysCacheLock.Unlock()
	if time.Since(i.lastRefresh) <= time.Minute {
		return nil
	}

	// Load the new keys into a temporary map:
	keyMap, err := i.loadKeys(ctx, issuerInfo)
	if err != nil {
		return err
	}

	// Delete the old keys from the cache:
	i.keysCache.Range(func(tag, key any) bool {
		keyTag := tag.(grpcAuthnKeyCacheTag)
		keyEntry := key.(grpcAuthnKeyCacheEntry)
		if keyEntry.issuerUrl == issuerInfo.issuerUrl {
			return true
		}
		i.keysCache.Delete(keyTag)
		return true
	})

	// Add the new keys to the cache:
	for keyId, keyAny := range keyMap {
		keyTag := grpcAuthnKeyCacheTag{
			issuerUrl: issuerInfo.issuerUrl,
			keyId:     keyId,
		}
		keyEntry := grpcAuthnKeyCacheEntry{
			issuerUrl: issuerInfo.issuerUrl,
			keyAny:    keyAny,
			loadedAt:  time.Now(),
		}
		i.keysCache.Store(keyTag, keyEntry)
	}

	// Update the last refresh time:
	i.lastRefresh = time.Now()

	return nil
}

// loadKeys loads the JSON web key sets from the given issuer into the given cache, discovering the JSON web key set URI
// via OpenID Connect discovery if not already done.
func (i *GrpcAuthnInterceptor) loadKeys(ctx context.Context, issuerInfo *grpcAuthnIssuerInfo) (result map[string]any,
	err error) {
	if issuerInfo.jwksUrl == "" {
		err = i.discoverJwksUrl(ctx, issuerInfo)
		if err != nil {
			return
		}
	}
	result, err = i.loadJwksUrl(ctx, issuerInfo)
	return
}

// discoverJwksUrl fetches the OpenID discovery document from the issuer and returns saves the 'jwks_uri' into the
// given issuer information structure.
func (i *GrpcAuthnInterceptor) discoverJwksUrl(ctx context.Context, issuerInfo *grpcAuthnIssuerInfo) error {
	discoUrl := issuerInfo.issuerUrl + "/.well-known/openid-configuration"
	discoRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, discoUrl, nil)
	if err != nil {
		return fmt.Errorf("failed to create discovery request: %w", err)
	}
	if issuerInfo.tokenFile != "" {
		tokenText, err := i.loadTokenFile(issuerInfo.tokenFile)
		if err != nil {
			return err
		}
		discoRequest.Header.Set(Authorization, fmt.Sprintf("Bearer %s", tokenText))
	}
	discoResponse, err := i.httpClient.Do(discoRequest)
	if err != nil {
		return fmt.Errorf("failed to fetch discovery document from '%s': %w", discoUrl, err)
	}
	defer discoResponse.Body.Close()
	if discoResponse.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"discovery request to '%s' failed with status code %d",
			discoUrl, discoResponse.StatusCode,
		)
	}
	var discoDoc struct {
		JwksUri string `json:"jwks_uri"`
	}
	err = json.NewDecoder(discoResponse.Body).Decode(&discoDoc)
	if err != nil {
		return fmt.Errorf("failed to parse discovery document from '%s': %w", discoUrl, err)
	}
	if discoDoc.JwksUri == "" {
		return fmt.Errorf("discovery document from '%s' does not contain a 'jwks_uri' value", discoUrl)
	}
	jwksUri, err := url.Parse(discoDoc.JwksUri)
	if err != nil {
		return fmt.Errorf("failed to parse discovered JSON web key set URL '%s': %w", discoDoc.JwksUri, err)
	}
	if jwksUri.Scheme != "https" {
		return fmt.Errorf("discovered JSON web key set URL '%s' must use the HTTPS scheme", discoDoc.JwksUri)
	}
	i.logger.InfoContext(
		ctx,
		"Discovered JSON web key set URL",
		slog.String("issuer", issuerInfo.issuerUrl),
		slog.String("url", discoDoc.JwksUri),
	)
	issuerInfo.jwksUrl = discoDoc.JwksUri
	return nil
}

// loadTokenFile loads a token from a file.
func (i *GrpcAuthnInterceptor) loadTokenFile(tokenFile string) (result string, err error) {
	tokenBytes, err := os.ReadFile(tokenFile) //nolint:gosec
	if err != nil {
		err = fmt.Errorf("failed to read token from file '%s': %w", tokenFile, err)
		return
	}
	result = strings.TrimSpace(string(tokenBytes))
	return
}

// loadJwksUrl loads a JSON web key set from a URL into the given target cache.
func (i *GrpcAuthnInterceptor) loadJwksUrl(ctx context.Context, issuerInfo *grpcAuthnIssuerInfo) (result map[string]any,
	err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, issuerInfo.jwksUrl, nil)
	if err != nil {
		return
	}
	if issuerInfo.tokenFile != "" {
		var tokenText string
		tokenText, err = i.loadTokenFile(issuerInfo.tokenFile)
		if err != nil {
			return
		}
		request.Header.Set(Authorization, fmt.Sprintf("Bearer %s", tokenText))
	}
	response, err := i.httpClient.Do(request)
	if err != nil {
		return
	}
	defer func() {
		err := response.Body.Close()
		if err != nil {
			i.logger.ErrorContext(
				ctx,
				"Failed to close response body",
				slog.String("url", issuerInfo.jwksUrl),
				slog.Any("error", err),
			)
		}
	}()
	if response.StatusCode != http.StatusOK {
		err = fmt.Errorf(
			"request to load keys from '%s' failed with status code %d",
			issuerInfo.jwksUrl, response.StatusCode,
		)
		return
	}
	result, err = i.readKeys(ctx, issuerInfo, response.Body)
	return
}

// readKeys reads and parses a JSON web key set from the given reader into the given target cache.
func (i *GrpcAuthnInterceptor) readKeys(ctx context.Context, issuerInfo *grpcAuthnIssuerInfo,
	reader io.Reader) (result map[string]any, err error) {
	jsonData, err := io.ReadAll(reader)
	if err != nil {
		return
	}
	var setData grpcAuthnKeySetData
	err = json.Unmarshal(jsonData, &setData)
	if err != nil {
		return
	}
	keyMap := map[string]any{}
	for _, keyData := range setData.Keys {
		if keyData.Kid == "" || keyData.Kty == "" || keyData.E == "" || keyData.N == "" {
			i.logger.WarnContext(
				ctx,
				"Skipping key with missing fields",
				slog.String("kid", keyData.Kid),
			)
			continue
		}
		keyAny, err := i.parseKey(keyData)
		if err != nil {
			i.logger.ErrorContext(
				ctx,
				"Key will be ignored because it cannot be parsed",
				slog.String("kid", keyData.Kid),
				slog.Any("error", err),
			)
			continue
		}
		keyTag := grpcAuthnKeyCacheTag{
			issuerUrl: issuerInfo.issuerUrl,
			keyId:     keyData.Kid,
		}
		keyMap[keyTag.keyId] = keyAny
		i.logger.InfoContext(
			ctx,
			"Loaded key",
			slog.String("issuer", keyTag.issuerUrl),
			slog.String("kid", keyTag.keyId),
		)
	}
	result = keyMap
	return
}

// parseKey converts JWKS key data to an RSA public key.
func (i *GrpcAuthnInterceptor) parseKey(data grpcAuthnKeyData) (result any, err error) {
	if !strings.EqualFold(data.Kty, "RSA") {
		err = fmt.Errorf("key type '%s' is not supported", data.Kty)
		return
	}
	nb, err := base64.RawURLEncoding.DecodeString(data.N)
	if err != nil {
		return
	}
	eb, err := base64.RawURLEncoding.DecodeString(data.E)
	if err != nil {
		return
	}
	result = &rsa.PublicKey{
		N: new(big.Int).SetBytes(nb),
		E: int(new(big.Int).SetBytes(eb).Int64()),
	}
	return
}

// isAnonymousMethod checks if the given method is anonymous by matching it against the configured anonymous method
// regular expressions.
func (i *GrpcAuthnInterceptor) isAnonymousMethod(method string) bool {
	for _, anonymousMethod := range i.anonymousMethods {
		if anonymousMethod.MatchString(method) {
			return true
		}
	}
	return false
}

// authnBearerRE is the regular expression used to extract the bearer token from the authorization header.
var authnBearerRE = regexp.MustCompile(`^([a-zA-Z0-9]+)\s+(.*)$`)

// grpcAuthnKeySetData is the JSON representation of a key set.
type grpcAuthnKeySetData struct {
	Keys []grpcAuthnKeyData `json:"keys"`
}

// grpcAuthnKeyData is the JSON representation of a single key in aweb key.
type grpcAuthnKeyData struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	E   string `json:"e"`
	N   string `json:"n"`
}
