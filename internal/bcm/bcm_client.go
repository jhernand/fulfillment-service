/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package bcm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client is the interface for interacting with the BCM API.
type Client interface {
	// GetDevices retrieves all devices from BCM.
	GetDevices(ctx context.Context) ([]Device, error)

	// GetRacksByUuids retrieves racks by their UUIDs.
	GetRacksByUuids(ctx context.Context, uuids []string) ([]Rack, error)

	// GetNetworksByUuids retrieves networks by their UUIDs.
	GetNetworksByUuids(ctx context.Context, uuids []string) ([]Network, error)

	// GetCategories retrieves all categories from BCM.
	GetCategories(ctx context.Context) ([]Category, error)

	// GetCategoriesByUuids retrieves categories by their UUIDs.
	GetCategoriesByUuids(ctx context.Context, uuids []string) ([]Category, error)
}

// Device represents a device in BCM.
type Device struct {
	Uuid         string         `json:"uuid"`
	ChildType    string         `json:"childType"`
	Hostname     string         `json:"hostname"`
	Interfaces   []Interface    `json:"interfaces"`
	BMCSettings  BMCSettings    `json:"bmcSettings"`
	RackPosition RackPosition   `json:"rackPosition"`
	Category     string         `json:"category"` // UUID of the category
	RawData      map[string]any `json:"-"`        // Store the raw JSON for debugging
}

// Interface represents a network interface.
type Interface struct {
	ChildType string `json:"childType"`
	Name      string `json:"name"`
	IP        string `json:"ip"`
}

// BMCSettings contains BMC authentication details.
type BMCSettings struct {
	UserName string `json:"userName"`
	Password string `json:"password"`
}

// RackPosition contains rack location information.
type RackPosition struct {
	Rack string `json:"rack"` // UUID of the rack
}

// Rack represents a rack in BCM.
type Rack struct {
	Name string `json:"name"`
}

// Network represents a network in BCM.
type Network struct {
	Name string `json:"name"`
}

// Category represents a category in BCM.
type Category struct {
	Uuid string `json:"uuid"`
	Name string `json:"name"`
}

// clientImpl is the implementation of the Client interface.
type clientImpl struct {
	logger     *slog.Logger
	httpClient *http.Client
	url        string
	timeout    time.Duration
}

// NewClient creates a new BCM client builder.
func NewClient() *ClientBuilder {
	return &ClientBuilder{}
}

// ClientBuilder is used to build BCM clients using the builder pattern.
type ClientBuilder struct {
	logger  *slog.Logger
	url     string
	certPEM []byte
	keyPEM  []byte
	caPool  *x509.CertPool
	timeout time.Duration
}

// SetLogger sets the logger for the client.
func (b *ClientBuilder) SetLogger(value *slog.Logger) *ClientBuilder {
	b.logger = value
	return b
}

// SetUrl sets the BCM API URL.
func (b *ClientBuilder) SetUrl(value string) *ClientBuilder {
	b.url = value
	return b
}

// SetCertFile sets the client certificate file.
func (b *ClientBuilder) SetCertFile(path string) *ClientBuilder {
	data, err := os.ReadFile(path)
	if err == nil {
		b.certPEM = data
	}
	return b
}

// SetKeyFile sets the client key file.
func (b *ClientBuilder) SetKeyFile(path string) *ClientBuilder {
	data, err := os.ReadFile(path)
	if err == nil {
		b.keyPEM = data
	}
	return b
}

// SetCaPool sets the CA certificate pool for TLS verification.
func (b *ClientBuilder) SetCaPool(value *x509.CertPool) *ClientBuilder {
	b.caPool = value
	return b
}

// SetTimeout sets the request timeout.
func (b *ClientBuilder) SetTimeout(value time.Duration) *ClientBuilder {
	b.timeout = value
	return b
}

// Build creates the BCM client.
func (b *ClientBuilder) Build() (Client, error) {
	// Check required parameters:
	if b.url == "" {
		return nil, fmt.Errorf("BCM URL is required")
	}
	if len(b.certPEM) == 0 {
		return nil, fmt.Errorf("client certificate is required")
	}
	if len(b.keyPEM) == 0 {
		return nil, fmt.Errorf("client key is required")
	}

	// Set defaults:
	logger := b.logger
	if logger == nil {
		logger = slog.Default()
	}
	timeout := b.timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Load the client certificate and key:
	cert, err := tls.X509KeyPair(b.certPEM, b.keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	// Create TLS configuration:
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		// We disable verification because BCM certificates may not have proper Authority Key Identifier extensions
		InsecureSkipVerify: true,
	}

	// Use CA pool if provided:
	if b.caPool != nil {
		tlsConfig.RootCAs = b.caPool
		tlsConfig.InsecureSkipVerify = false
	}

	// Create HTTP client:
	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	// Create and return the client:
	return &clientImpl{
		logger:     logger,
		httpClient: httpClient,
		url:        strings.TrimSuffix(b.url, "/") + "/json",
		timeout:    timeout,
	}, nil
}

// GetDevices retrieves all devices from BCM.
func (c *clientImpl) GetDevices(ctx context.Context) ([]Device, error) {
	c.logger.DebugContext(ctx, "Fetching devices from BCM")

	// Prepare the request:
	request := map[string]any{
		"service": "cmdevice",
		"call":    "getDevices",
		"minify":  true,
		"args":    []any{},
	}

	// Make the API call:
	var devices []Device
	err := c.call(ctx, request, &devices)
	if err != nil {
		return nil, fmt.Errorf("failed to get devices: %w", err)
	}

	c.logger.DebugContext(
		ctx,
		"Fetched devices from BCM",
		slog.Int("count", len(devices)),
	)

	return devices, nil
}

// GetRacksByUuids retrieves racks by their UUIDs.
func (c *clientImpl) GetRacksByUuids(ctx context.Context, uuids []string) ([]Rack, error) {
	if len(uuids) == 0 {
		return []Rack{}, nil
	}

	c.logger.DebugContext(
		ctx,
		"Fetching racks from BCM",
		slog.Int("count", len(uuids)),
	)

	// Prepare the request:
	request := map[string]any{
		"service": "cmpart",
		"call":    "getRacksByUuids",
		"minify":  true,
		"args":    []any{uuids},
	}

	// Make the API call:
	var racks []Rack
	err := c.call(ctx, request, &racks)
	if err != nil {
		return nil, fmt.Errorf("failed to get racks: %w", err)
	}

	return racks, nil
}

// GetNetworksByUuids retrieves networks by their UUIDs.
func (c *clientImpl) GetNetworksByUuids(ctx context.Context, uuids []string) ([]Network, error) {
	if len(uuids) == 0 {
		return []Network{}, nil
	}

	c.logger.DebugContext(
		ctx,
		"Fetching networks from BCM",
		slog.Int("count", len(uuids)),
	)

	// Prepare the request:
	request := map[string]any{
		"service": "cmnet",
		"call":    "getNetworksByUuids",
		"minify":  true,
		"args":    []any{uuids},
	}

	// Make the API call:
	var networks []Network
	err := c.call(ctx, request, &networks)
	if err != nil {
		return nil, fmt.Errorf("failed to get networks: %w", err)
	}

	return networks, nil
}

// GetCategories retrieves all categories from BCM.
func (c *clientImpl) GetCategories(ctx context.Context) ([]Category, error) {
	c.logger.DebugContext(ctx, "Fetching categories from BCM")

	// Prepare the request to search for Category entities:
	request := map[string]any{
		"service": "cmdevice",
		"call":    "getCategories",
		"minify":  true,
		"args":    []any{},
	}

	// Make the API call:
	var categories []Category
	err := c.call(ctx, request, &categories)
	if err != nil {
		return nil, fmt.Errorf("failed to get categories: %w", err)
	}

	c.logger.DebugContext(
		ctx,
		"Fetched categories from BCM",
		slog.Int("count", len(categories)),
	)

	return categories, nil
}

// GetCategoriesByUuids retrieves categories by their UUIDs.
func (c *clientImpl) GetCategoriesByUuids(ctx context.Context, uuids []string) ([]Category, error) {
	if len(uuids) == 0 {
		return []Category{}, nil
	}

	c.logger.DebugContext(
		ctx,
		"Fetching categories from BCM",
		slog.Int("count", len(uuids)),
	)

	// Prepare the request:
	request := map[string]any{
		"service": "cmentity",
		"call":    "getCategoriesByUuids",
		"minify":  true,
		"args":    []any{uuids},
	}

	// Make the API call:
	var categories []Category
	err := c.call(ctx, request, &categories)
	if err != nil {
		return nil, fmt.Errorf("failed to get categories: %w", err)
	}

	return categories, nil
}

// call makes a generic API call to BCM.
func (c *clientImpl) call(ctx context.Context, request map[string]any, result any) error {
	// Marshal the request:
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the HTTP request:
	req, err := http.NewRequestWithContext(ctx, "POST", c.url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Execute the request:
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check the response status:
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("BCM API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse the response:
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	c.logger.DebugContext(
		ctx,
		"Response from BCM",
		slog.String("body", string(bodyBytes)),
	)

	err = json.Unmarshal(bodyBytes, result)
	if err != nil {
		c.logger.ErrorContext(
			ctx,
			"Failed to parse response",
			slog.String("body", string(bodyBytes)),
			slog.Any("error", err),
		)
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return nil
}
