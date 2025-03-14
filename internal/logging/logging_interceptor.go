/*
Copyright (c) 2025 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package logging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	grpcpeer "google.golang.org/grpc/peer"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// InterceptorBuilder contains the data and logic needed to build an interceptor that writes to the log the details of
// calls. Don't create instances of this type directly, use the NewInterceptor function instead.
type InterceptorBuilder struct {
	logger  *slog.Logger
	headers bool
	bodies  bool
	redact  bool
	flags   *pflag.FlagSet
}

// Interceptor contains the data needed by the Interceptor, like the logger and settings.
type Interceptor struct {
	logger  *slog.Logger
	headers bool
	bodies  bool
	redact  bool
}

// NewInterceptor creates a builder that can then be used to configure and create a logging interceptor.
func NewInterceptor() *InterceptorBuilder {
	return &InterceptorBuilder{
		redact: true,
	}
}

// SetLogger sets the logger that will be used to write to the log. This is mandatory.
func (b *InterceptorBuilder) SetLogger(value *slog.Logger) *InterceptorBuilder {
	b.logger = value
	return b
}

// SetHeaders indicates if headers should be included in log messages. The default is to not include them.
func (b *InterceptorBuilder) SetHeaders(value bool) *InterceptorBuilder {
	b.headers = value
	return b
}

// SetBodies indicates if details about the request and response bodies should be included in log messages. The default
// is to not include them.
func (b *InterceptorBuilder) SetBodies(value bool) *InterceptorBuilder {
	b.bodies = value
	return b
}

// SetRedact indicates if security sensitive information should be redacted. The default is true.
func (b *InterceptorBuilder) SetRedact(value bool) *InterceptorBuilder {
	b.redact = value
	return b
}

// SetFlags sets the command line flags that should be used to configure the interceptor. This is optional.
func (b *InterceptorBuilder) SetFlags(flags *pflag.FlagSet) *InterceptorBuilder {
	b.flags = flags
	if flags != nil {
		if flags.Changed(headersFlagName) {
			value, err := flags.GetBool(headersFlagName)
			if err == nil {
				b.SetHeaders(value)
			}
		}
		if flags.Changed(bodiesFlagName) {
			value, err := flags.GetBool(bodiesFlagName)
			if err == nil {
				b.SetBodies(value)
			}
		}
	}
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
		logger:  b.logger,
		headers: b.headers,
		bodies:  b.bodies,
		redact:  b.redact,
	}
	return
}

// UnaryServer is the unary server interceptor function.
func (i *Interceptor) UnaryServer(ctx context.Context, request any, info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (response any, err error) {
	// Ignore reflection calls:
	if i.isReflectionMethod(info.FullMethod) {
		response, err = handler(ctx, request)
		return
	}

	// The processing here is expensive, so better if we avoid it completely when debug is disabled:
	if !i.logger.Enabled(ctx, slog.LevelDebug) {
		response, err = handler(ctx, request)
		return
	}

	// Get the time before calling the handler so that we can later compute the duration of the call:
	timeBefore := time.Now()

	// Write the details of the request:
	methodField := slog.String("method", info.FullMethod)
	requestFields := []any{
		methodField,
	}
	peerInfo, ok := grpcpeer.FromContext(ctx)
	if ok {
		peerAddr := peerInfo.Addr
		if peerAddr != nil {
			peerAddrField := slog.String("address", peerAddr.String())
			requestFields = append(requestFields, peerAddrField)
		}
	}
	if i.headers {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			if i.redact {
				md = i.redactMD(md)
			}
			mdField := slog.Any("metadata", md)
			requestFields = append(requestFields, mdField)
		}
	}
	if i.bodies && request != nil {
		bodyField, ok := i.dumpMessage(ctx, "request", request)
		if ok {
			requestFields = append(requestFields, bodyField)
		}
	}
	i.logger.DebugContext(ctx, "Received unary request", requestFields...)

	// Call the handler:
	response, err = handler(ctx, request)

	// Write the details of the response:
	timeElapsed := time.Since(timeBefore)
	timeField := slog.Duration("duration", timeElapsed)
	codeField := slog.String("code", grpcstatus.Code(err).String())
	responseFields := []any{
		methodField,
		timeField,
		codeField,
	}
	if i.bodies && response != nil {
		bodyField, ok := i.dumpMessage(ctx, "response", response)
		if ok {
			responseFields = append(responseFields, bodyField)
		}
	}
	if err != nil {
		errField := slog.String("error", err.Error())
		responseFields = append(responseFields, errField)
	}
	i.logger.DebugContext(ctx, "Sent unary response", responseFields...)

	return
}

// StreamServer is the stream server interceptor function.
func (i *Interceptor) StreamServer(server any, stream grpc.ServerStream, info *grpc.StreamServerInfo,
	handler grpc.StreamHandler) error {
	// Ignore reflection calls:
	if i.isReflectionMethod(info.FullMethod) {
		return handler(server, stream)
	}

	// The processing here is expensive, so better if we avoid it completely when debug is disabled:
	ctx := stream.Context()
	if !i.logger.Enabled(ctx, slog.LevelDebug) {
		return handler(server, stream)
	}

	// Get the time before calling the handler so that we can later compute the duration of the call:
	timeBefore := time.Now()

	// Write the details of the request:
	methodField := slog.String("method", info.FullMethod)
	requestFields := []any{
		methodField,
	}
	peerInfo, ok := grpcpeer.FromContext(ctx)
	if ok {
		peerAddr := peerInfo.Addr
		if peerAddr != nil {
			peerAddrField := slog.String("address", peerAddr.String())
			requestFields = append(requestFields, peerAddrField)
		}
	}
	if i.headers {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			if i.redact {
				md = i.redactMD(md)
			}
			mdField := slog.Any("metadata", md)
			requestFields = append(requestFields, mdField)
		}
	}
	i.logger.DebugContext(ctx, "Received stream start request", requestFields...)

	// Wrap the stream so that we can log the details of the messages exchanged:
	stream = &interceptorStream{
		parent: i,
		logger: i.logger,
		// request: request,
		stream: stream,
	}
	err := handler(server, stream)

	// Write the details of the response:
	timeElapsed := time.Since(timeBefore)
	timeField := slog.Duration("duration", timeElapsed)
	responseFields := []any{
		methodField,
		timeField,
	}
	if err != nil {
		errField := slog.String("error", err.Error())
		responseFields = append(responseFields, errField)
	}
	i.logger.DebugContext(ctx, "Sent stream stop response", responseFields...)

	return err
}

func (i *Interceptor) isReflectionMethod(method string) bool {
	return strings.HasPrefix(method, "/grpc.reflection.")
}

// redactMetadata generates a copy of the given metadata that doesn't contain security sensitive data, like
// authentication credentials.
func (i *Interceptor) redactMD(md metadata.MD) metadata.MD {
	result := make(metadata.MD)
	for key, values := range md {
		redacted := slices.Clone(values)
		for j, value := range values {
			redacted[j] = i.redactMDValue(key, value)
		}
		result[key] = redacted
	}
	return result
}

func (i *Interceptor) redactMDValue(key string, value string) string {
	switch key {
	case "authorization":
		return i.redactMDAuthorization(value)
	default:
		return value
	}
}

func (i *Interceptor) redactMDAuthorization(value string) string {
	position := strings.Index(value, " ")
	if position == -1 {
		return redactMark
	}
	scheme := value[0:position]
	return fmt.Sprintf("%s %s", scheme, redactMark)
}

// dumpMessage tries to covert the given message to something that can be added to the log and returns the corresponding
// log fields.
func (i *Interceptor) dumpMessage(ctx context.Context, key string, value any) (field any, ok bool) {
	switch message := value.(type) {
	case proto.Message:
		bytes, err := protojson.Marshal(message)
		if err != nil {
			i.logger.ErrorContext(
				ctx,
				"Failed to marshal protocol buffers message",
				slog.String("error", err.Error()),
			)
			return
		}
		var data any
		err = json.Unmarshal(bytes, &data)
		if err != nil {
			i.logger.ErrorContext(
				ctx,
				"Failed to unmarshal protocol buffers message",
				slog.String("error", err.Error()),
			)
			return
		}
		ok = true
		field = slog.Any(key, data)
	default:
		i.logger.ErrorContext(
			ctx,
			"Failed to dump value because it isn't a protocol buffers message",
			slog.String("type", fmt.Sprintf("%T", value)),
		)
	}
	return
}

type interceptorStream struct {
	parent *Interceptor
	logger *slog.Logger
	stream grpc.ServerStream
}

func (s *interceptorStream) Context() context.Context {
	return s.stream.Context()
}

func (s *interceptorStream) RecvMsg(message any) error {
	// Return inmediately if this is the end of the stream:
	err := s.stream.RecvMsg(message)
	if errors.Is(err, io.EOF) {
		return err
	}

	// Write the details of the received message:
	ctx := s.stream.Context()
	var messageFields []any
	if s.parent.bodies && message != nil {
		bodyField, ok := s.parent.dumpMessage(ctx, "message", message)
		if ok {
			messageFields = append(messageFields, bodyField)
		}
	}
	if err != nil {
		errField := slog.String("error", err.Error())
		messageFields = append(messageFields, errField)
	}
	s.logger.DebugContext(ctx, "Received stream message", messageFields...)

	return err
}

func (s *interceptorStream) SendHeader(md metadata.MD) error {
	// Call the stream:
	err := s.stream.SendHeader(md)

	// Write the details of the sent header:
	ctx := s.stream.Context()
	var metadataFields []any
	if s.parent.headers {
		if s.parent.redact {
			md = s.parent.redactMD(md)
		}
		mdField := slog.Any("metadata", md)
		metadataFields = append(metadataFields, mdField)
	}
	if err != nil {
		errField := slog.String("error", err.Error())
		metadataFields = append(metadataFields, errField)
	}
	s.logger.DebugContext(ctx, "Sent stream metadata", metadataFields...)

	return err
}

func (s *interceptorStream) SendMsg(message any) error {
	// Call the stream:
	err := s.stream.SendMsg(message)

	// Write the details of the sent message:
	ctx := s.stream.Context()
	var messageFields []any
	if s.parent.bodies && message != nil {
		bodyField, ok := s.parent.dumpMessage(ctx, "message", message)
		if ok {
			messageFields = append(messageFields, bodyField)
		}
	}
	if err != nil {
		errField := slog.String("error", err.Error())
		messageFields = append(messageFields, errField)
	}
	s.logger.DebugContext(ctx, "Sent stream message", messageFields...)

	return err
}

func (s *interceptorStream) SetHeader(md metadata.MD) error {
	// Call the stream:
	err := s.stream.SetHeader(md)

	// Write the details of the set header:
	ctx := s.stream.Context()
	var metadataFields []any
	if s.parent.headers {
		if s.parent.redact {
			md = s.parent.redactMD(md)
		}
		mdField := slog.Any("metadata", md)
		metadataFields = append(metadataFields, mdField)
	}
	if err != nil {
		errField := slog.String("error", err.Error())
		metadataFields = append(metadataFields, errField)
	}
	s.logger.DebugContext(ctx, "Set stream metadata", metadataFields...)

	return err
}

func (s *interceptorStream) SetTrailer(md metadata.MD) {
	// Call the stream:
	s.stream.SetTrailer(md)

	// Write the details of the set header:
	ctx := s.stream.Context()
	var metadataFields []any
	if s.parent.headers {
		if s.parent.redact {
			md = s.parent.redactMD(md)
		}
		mdField := slog.Any("metadata", md)
		metadataFields = append(metadataFields, mdField)
	}
	s.logger.DebugContext(ctx, "Set stream metadata", metadataFields...)
}
