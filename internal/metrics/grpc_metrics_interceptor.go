/*
Copyright (c) 2025 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package metrics

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	grpcstatus "google.golang.org/grpc/status"
)

// GrpcMetricsInterceptorBuilder contains the data and logic needed to build an interceptor that collects the following
// metrics for both server and client gRPC calls:
//
//	<subsystem>_call_count - Number of gRPC method calls.
//	<subsystem>_call_duration_sum - Total time to process gRPC method calls, in seconds.
//	<subsystem>_call_duration_count - Total number of gRPC method calls measured.
//	<subsystem>_call_duration_bucket - Number of gRPC method calls organized in buckets.
//
// To set the subsystem prefix use the SetSubsystem method.
//
// The duration buckets metrics contain a `le` label that indicates the upper bound. For example if the `le` label is `1`
// then the value will be the number of method calls that were processed in less than one second.
//
// The metrics will have the following labels:
//
//	method - Full name of the gRPC method, for example `/fulfillment.v1.ClustersService/List`.
//	code - gRPC response code, for example `OK` or `INTERNAL`.
//
// To calculate the average method call duration during the last 10 minutes, for example, use a Prometheus expression
// like this:
//
//	rate(grpc_call_duration_sum[10m]) / rate(grpc_call_duration_count[10m])
//
// The value of the `code` label will be empty when sending the request failed without a response code, for example if
// it wasn't possible to open the connection, or if there was a timeout waiting for the response.
//
// Don't create instances of this type directly, use the NewGrpcMetricsInterceptor function instead.
type GrpcMetricsInterceptorBuilder struct {
	logger     *slog.Logger
	subsystem  string
	registerer prometheus.Registerer
}

// GrpcMetricsInterceptor contains the data needed by the interceptor.
type GrpcMetricsInterceptor struct {
	logger       *slog.Logger
	callCount    *prometheus.CounterVec
	callDuration *prometheus.HistogramVec
}

// NewGrpcMetricsInterceptor creates a builder that can then be used to configure and create a metrics interceptor.
func NewGrpcMetricsInterceptor() *GrpcMetricsInterceptorBuilder {
	return &GrpcMetricsInterceptorBuilder{
		registerer: prometheus.DefaultRegisterer,
	}
}

// SetLogger sets the logger that will be used to write to the log. This is mandatory.
func (b *GrpcMetricsInterceptorBuilder) SetLogger(value *slog.Logger) *GrpcMetricsInterceptorBuilder {
	b.logger = value
	return b
}

// SetSubsystem sets the name of the subsystem that will be used by to register the metrics with Prometheus. For example,
// if the value is `grpc` then the following metrics will be registered:
//
//	grpc_call_count - Number of gRPC method calls.
//	grpc_call_duration_sum - Total time to process gRPC method calls, in seconds.
//	grpc_call_duration_count - Total number of gRPC method calls measured.
//	grpc_call_duration_bucket - Number of gRPC method calls organized in buckets.
//
// This is mandatory.
func (b *GrpcMetricsInterceptorBuilder) SetSubsystem(value string) *GrpcMetricsInterceptorBuilder {
	b.subsystem = value
	return b
}

// SetRegisterer sets the Prometheus registerer that will be used to register the metrics. The default is to use the
// default Prometheus registerer and there is usually no need to change that. This is intended for unit tests, where it
// is convenient to have a registerer that doesn't interfere with the rest of the system.
func (b *GrpcMetricsInterceptorBuilder) SetRegisterer(value prometheus.Registerer) *GrpcMetricsInterceptorBuilder {
	b.registerer = value
	return b
}

// Build uses the data stored in the builder to create and configure a new interceptor.
func (b *GrpcMetricsInterceptorBuilder) Build() (result *GrpcMetricsInterceptor, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.subsystem == "" {
		err = errors.New("subsystem is mandatory")
		return
	}

	// Register the request count metric:
	callCount := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: b.subsystem,
			Name:      "call_count",
			Help:      "Number of calls processed.",
		},
		callLabelNames,
	)
	err = b.registerer.Register(callCount)
	if err != nil {
		registered, ok := err.(prometheus.AlreadyRegisteredError)
		if ok {
			callCount = registered.ExistingCollector.(*prometheus.CounterVec)
			err = nil
		} else {
			return
		}
	}

	// Register the request duration metric:
	callDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: b.subsystem,
			Name:      "call_duration",
			Help:      "Call duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		callLabelNames,
	)
	err = b.registerer.Register(callDuration)
	if err != nil {
		registered, ok := err.(prometheus.AlreadyRegisteredError)
		if ok {
			callDuration = registered.ExistingCollector.(*prometheus.HistogramVec)
			err = nil
		} else {
			return
		}
	}

	// Create and populate the object:
	result = &GrpcMetricsInterceptor{
		logger:       b.logger,
		callCount:    callCount,
		callDuration: callDuration,
	}
	return
}

// UnaryServer is the unary server interceptor function that collects metrics.
func (i *GrpcMetricsInterceptor) UnaryServer(ctx context.Context, request any, info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (response any, err error) {
	before := time.Now()
	response, err = handler(ctx, request)
	elapsed := time.Since(before)
	code := grpcstatus.Code(err).String()
	i.callCount.WithLabelValues(info.FullMethod, code).Inc()
	i.callDuration.WithLabelValues(info.FullMethod, code).Observe(elapsed.Seconds())
	return
}

// StreamServer is the stream server interceptor function that collects metrics.
func (i *GrpcMetricsInterceptor) StreamServer(server any, stream grpc.ServerStream, info *grpc.StreamServerInfo,
	handler grpc.StreamHandler) error {
	before := time.Now()
	err := handler(server, stream)
	elapsed := time.Since(before)
	code := grpcstatus.Code(err).String()
	i.callCount.WithLabelValues(info.FullMethod, code).Inc()
	i.callDuration.WithLabelValues(info.FullMethod, code).Observe(elapsed.Seconds())
	return err
}

// UnaryClient is the unary client interceptor function that collects metrics.
func (i *GrpcMetricsInterceptor) UnaryClient(ctx context.Context, method string, request, response any,
	cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	before := time.Now()
	err := invoker(ctx, method, request, response, cc, opts...)
	elapsed := time.Since(before)
	code := grpcstatus.Code(err).String()
	i.callCount.WithLabelValues(method, code).Inc()
	i.callDuration.WithLabelValues(method, code).Observe(elapsed.Seconds())
	return err
}

// StreamClient is the stream client interceptor function that collects metrics.
func (i *GrpcMetricsInterceptor) StreamClient(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
	method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	before := time.Now()
	stream, err := streamer(ctx, desc, cc, method, opts...)
	elapsed := time.Since(before)
	code := status.Code(err).String()
	i.callCount.WithLabelValues(method, code).Inc()
	i.callDuration.WithLabelValues(method, code).Observe(elapsed.Seconds())
	return stream, err
}
