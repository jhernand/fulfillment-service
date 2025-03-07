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
	"errors"
	"log/slog"

	"github.com/spf13/pflag"
	"google.golang.org/grpc"
)

// InterceptorBuilder contains the data and logic needed to build an interceptor that checks authentication. Don't
// create instances of this type directly, use the NewInterceptor function instead.
type InterceptorBuilder struct {
	logger *slog.Logger
	flags  *pflag.FlagSet
}

// Interceptor contains the data needed by the interceptor.
type Interceptor struct {
	logger *slog.Logger
}

// NewInterceptor creates a builder that can then be used to configure and create an authentication interceptor.
func NewInterceptor() *InterceptorBuilder {
	return &InterceptorBuilder{}
}

// SetLogger sets the logger that will be used to write to the log. This is mandatory.
func (b *InterceptorBuilder) SetLogger(value *slog.Logger) *InterceptorBuilder {
	b.logger = value
	return b
}

// SetFlags sets the command line flags that should be used to configure the interceptor. This is optional.
func (b *InterceptorBuilder) SetFlags(flags *pflag.FlagSet) *InterceptorBuilder {
	b.flags = flags
	return b
}

// Build uses the data stored in the builder to create and configure a new interceptor.
func (b *InterceptorBuilder) Build() (result *Interceptor, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}

	// Create and populate the object:
	result = &Interceptor{
		logger: b.logger,
	}
	return
}

// UnaryServer is the unary server interceptor function.
func (i *Interceptor) UnaryServer(ctx context.Context, request any, info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (response any, err error) {
	// TODO: Implement this.
	response, err = handler(ctx, request)
	return
}

// StreamServer is the stream server interceptor function.
func (i *Interceptor) StreamServer(server any, stream grpc.ServerStream, info *grpc.StreamServerInfo,
	handler grpc.StreamHandler) error {
	// TODO: Implement this.
	return handler(server, stream)
}
