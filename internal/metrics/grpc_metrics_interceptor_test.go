/*
Copyright (c) 2025 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

// This file contains tests for the metrics transport interceptor.

package metrics

import (
	"context"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/ginkgo/v2/dsl/table"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	. "github.com/innabox/fulfillment-service/internal/testing"
)

var _ = Describe("Metrics interceptor", func() {
	Describe("Creation", func() {
		It("Can be created with all the mandatory parameters", func() {
			interceptor, err := NewGrpcMetricsInterceptor().
				SetLogger(logger).
				SetSubsystem("my").
				Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(interceptor).ToNot(BeNil())
		})

		It("Can't be created without a logger", func() {
			interceptor, err := NewGrpcMetricsInterceptor().
				SetSubsystem("my").
				Build()
			Expect(err).To(HaveOccurred())
			Expect(interceptor).To(BeNil())
			Expect(err).To(MatchError("logger is mandatory"))
		})

		It("Can't be created without a subsystem", func() {
			interceptor, err := NewGrpcMetricsInterceptor().
				SetLogger(logger).
				Build()
			Expect(err).To(HaveOccurred())
			Expect(interceptor).To(BeNil())
			Expect(err).To(MatchError("subsystem is mandatory"))
		})
	})

	Describe("Behaviour", func() {
		var (
			ctx         context.Context
			server      *MetricsServer
			interceptor *GrpcMetricsInterceptor
		)

		BeforeEach(func() {
			var err error

			// Create the context:
			ctx = context.Background()

			// Start the metrics server:
			server = NewMetricsServer()

			// Create the interceptor:
			interceptor, err = NewGrpcMetricsInterceptor().
				SetLogger(logger).
				SetSubsystem("my").
				SetRegisterer(server.Registry()).
				Build()
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			// Stop the metrics server:
			server.Close()
		})

		It("Calls unary server handler", func() {
			// Prepare the handler:
			called := false
			handler := func(context.Context, any) (any, error) {
				called = true
				return nil, nil
			}

			// Call the interceptor:
			info := &grpc.UnaryServerInfo{
				FullMethod: "/my_package.MyService/MyMethod",
			}
			interceptor.UnaryServer(ctx, nil, info, handler)

			// Check that the handler was called:
			Expect(called).To(BeTrue())
		})

		Describe("Call count", func() {
			It("Honours subsystem", func() {
				// Prepare the handler:
				handler := func(context.Context, any) (any, error) {
					return nil, nil
				}

				// Call the interceptor:
				info := &grpc.UnaryServerInfo{
					FullMethod: "/my_package.MyService/MyMethod",
				}
				interceptor.UnaryServer(ctx, nil, info, handler)

				// Verify the metrics:
				metrics := server.Metrics()
				Expect(metrics).To(MatchLine(`^my_call_count\{.*\} .*$`))
			})

			It("Includes method label", func() {
				// Prepare the handler:
				handler := func(context.Context, any) (any, error) {
					return nil, nil
				}

				// Call the interceptor:
				info := &grpc.UnaryServerInfo{
					FullMethod: "/my_package.MyService/MyMethod",
				}
				interceptor.UnaryServer(ctx, nil, info, handler)

				// Verify the metrics:
				metrics := server.Metrics()
				Expect(metrics).To(MatchLine(`^my_call_count\{.*method="/my_package.MyService/MyMethod".*\} .*$`))
			})

			DescribeTable(
				"Includes code label",
				func(code grpccodes.Code, label string) {
					// Prepare the handler:
					handler := func(context.Context, any) (any, error) {
						return nil, grpcstatus.Error(code, "My message")
					}

					// Call the interceptor:
					info := &grpc.UnaryServerInfo{
						FullMethod: "/my_package.MyService/MyMethod",
					}
					interceptor.UnaryServer(ctx, nil, info, handler)

					// Verify the metrics:
					metrics := server.Metrics()
					Expect(metrics).To(MatchLine(`^my_call_count\{.*code="%s".*\} .*$`, label))
				},
				Entry(
					"OK",
					grpccodes.OK,
					"OK",
				),
				Entry(
					"Canceled",
					grpccodes.Canceled,
					"Canceled",
				),
				Entry(
					"Unknown",
					grpccodes.Unknown,
					"Unknown",
				),
				Entry(
					"Invalid argument",
					grpccodes.InvalidArgument,
					"InvalidArgument",
				),
				Entry(
					"Deadline exceeded",
					grpccodes.DeadlineExceeded,
					"DeadlineExceeded",
				),
				Entry(
					"Not found",
					grpccodes.NotFound,
					"NotFound",
				),
				Entry(
					"Already exists",
					grpccodes.AlreadyExists,
					"AlreadyExists",
				),
				Entry(
					"Permission denied",
					grpccodes.PermissionDenied,
					"PermissionDenied",
				),
				Entry(
					"Resource exhausted",
					grpccodes.ResourceExhausted,
					"ResourceExhausted",
				),
				Entry(
					"Failed precondition",
					grpccodes.FailedPrecondition,
					"FailedPrecondition",
				),
				Entry(
					"Aborted",
					grpccodes.Aborted,
					"Aborted",
				),
				Entry(
					"Out of range",
					grpccodes.OutOfRange,
					"OutOfRange",
				),
				Entry(
					"Unimplemented",
					grpccodes.Unimplemented,
					"Unimplemented",
				),
				Entry(
					"Internal",
					grpccodes.Internal,
					"Internal",
				),
			)

			It("Includes code label", func() {
				// Prepare the handler:
				handler := func(context.Context, any) (any, error) {
					return nil, nil
				}

				// Call the interceptor:
				info := &grpc.UnaryServerInfo{
					FullMethod: "/my_package.MyService/MyMethod",
				}
				interceptor.UnaryServer(ctx, nil, info, handler)

				// Verify the metrics:
				metrics := server.Metrics()
				Expect(metrics).To(MatchLine(`^my_call_count\{.*method="/my_package.MyService/MyMethod".*\} .*$`))
			})
		})

		/*
			Describe("Request count", func() {
				It("Honours subsystem", func() {
					// Prepare the handler:
					handler = interceptor.UnaryServer(RespondWith(http.StatusOK, nil))

					// Send the request:
					Send(http.MethodGet, "/api")

					// Verify the metrics:
					metrics := server.Metrics()
					Expect(metrics).To(MatchLine(`^my_request_count\{.*\} .*$`))
				})

				DescribeTable(
					"Counts correctly",
					func(count int) {
						// Prepare the handler:
						handler = interceptor.UnaryServer(RespondWith(http.StatusOK, nil))

						// Send the requests:
						for i := 0; i < count; i++ {
							Send(http.MethodGet, "/api")
						}

						// Verify the metrics:
						metrics := server.Metrics()
						Expect(metrics).To(MatchLine(`^\w+_request_count\{.*\} %d$`, count))
					},
					Entry("One", 1),
					Entry("Two", 2),
					Entry("Trhee", 3),
				)

				DescribeTable(
					"Includes method label",
					func(method string) {
						// Prepare the handler:
						handler = interceptor.UnaryServer(RespondWith(http.StatusOK, nil))

						// Send the requests:
						Send(method, "/api")

						// Verify the metrics:
						metrics := server.Metrics()
						Expect(metrics).To(MatchLine(`^\w+_request_count\{.*method="%s".*\} .*$`, method))
					},
					Entry("GET", http.MethodGet),
					Entry("POST", http.MethodPost),
					Entry("PATCH", http.MethodPatch),
					Entry("DELETE", http.MethodDelete),
				)

				DescribeTable(
					"Includes path label",
					func(path, label string) {
						// Prepare the handler:
						handler = interceptor.UnaryServer(RespondWith(http.StatusOK, nil))

						// Send the requests:
						Send(http.MethodGet, path)

						// Verify the metrics:
						metrics := server.Metrics()
						Expect(metrics).To(MatchLine(`^\w+_request_count\{.*path="%s".*\} .*$`, label))
					},
					Entry(
						"Empty",
						"",
						"/-",
					),
					Entry(
						"One slash",
						"/",
						"/-",
					),
					Entry(
						"Two slashes",
						"//",
						"/-",
					),
					Entry(
						"Tree slashes",
						"///",
						"/-",
					),
					Entry(
						"API root",
						"/api",
						"/api",
					),
					Entry(
						"API root with trailing slash",
						"/api/",
						"/api",
					),
					Entry(
						"Unknown root",
						"/junk/",
						"/-",
					),
					Entry(
						"Service root",
						"/api/clusters_mgmt",
						"/api/clusters_mgmt",
					),
					Entry(
						"Unknown service root",
						"/api/junk",
						"/-",
					),
					Entry(
						"Version root",
						"/api/clusters_mgmt/v1",
						"/api/clusters_mgmt/v1",
					),
					Entry(
						"Unknown version root",
						"/api/junk/v1",
						"/-",
					),
					Entry(
						"Collection",
						"/api/clusters_mgmt/v1/clusters",
						"/api/clusters_mgmt/v1/clusters",
					),
					Entry(
						"Unknown collection",
						"/api/clusters_mgmt/v1/junk",
						"/-",
					),
					Entry(
						"Collection item",
						"/api/clusters_mgmt/v1/clusters/123",
						"/api/clusters_mgmt/v1/clusters/-",
					),
					Entry(
						"Collection item action",
						"/api/clusters_mgmt/v1/clusters/123/hibernate",
						"/api/clusters_mgmt/v1/clusters/-/hibernate",
					),
					Entry(
						"Unknown collection item action",
						"/api/clusters_mgmt/v1/clusters/123/junk",
						"/-",
					),
					Entry(
						"Subcollection",
						"/api/clusters_mgmt/v1/clusters/123/groups",
						"/api/clusters_mgmt/v1/clusters/-/groups",
					),
					Entry(
						"Unknown subcollection",
						"/api/clusters_mgmt/v1/clusters/123/junks",
						"/-",
					),
					Entry(
						"Subcollection item",
						"/api/clusters_mgmt/v1/clusters/123/groups/456",
						"/api/clusters_mgmt/v1/clusters/-/groups/-",
					),
					Entry(
						"Too long",
						"/api/clusters_mgmt/v1/clusters/123/groups/456/junk",
						"/-",
					),
					Entry(
						"Explicitly specified path",
						"/my/path",
						"/my/path",
					),
					Entry(
						"Unknown path",
						"/your/path",
						"/-",
					),
				)

				DescribeTable(
					"Includes code label",
					func(code int) {
						// Prepare the handler:
						handler = interceptor.Wrap(RespondWith(code, nil))

						// Send the requests:
						Send(http.MethodGet, "/api")

						// Verify the metrics:
						metrics := server.Metrics()
						Expect(metrics).To(MatchLine(`^\w+_request_count\{.*code="%d".*\} .*$`, code))
					},
					Entry("200", http.StatusOK),
					Entry("201", http.StatusCreated),
					Entry("202", http.StatusAccepted),
					Entry("401", http.StatusUnauthorized),
					Entry("404", http.StatusNotFound),
					Entry("500", http.StatusInternalServerError),
				)

				DescribeTable(
					"Includes API service label",
					func(path, label string) {
						// Prepare the handler:
						handler = interceptor.Wrap(RespondWith(http.StatusOK, nil))

						// Send the requests:
						Send(http.MethodGet, path)

						// Verify the metrics:
						metrics := server.Metrics()
						Expect(metrics).To(MatchLine(`^\w+_request_count\{.*apiservice="%s".*\} .*$`, label))
					},
					Entry(
						"Empty",
						"",
						"",
					),
					Entry(
						"Root",
						"/",
						"",
					),
					Entry(
						"Clusters root",
						"/api/clusters_mgmt",
						"ocm-clusters-service",
					),
					Entry(
						"Clusters version",
						"/api/clusters_mgmt/v1",
						"ocm-clusters-service",
					),
					Entry(
						"Clusters collection",
						"/api/clusters_mgmt/v1/clusters",
						"ocm-clusters-service",
					),
					Entry(
						"Clusters item",
						"/api/clusters_mgmt/v1/clusters/123",
						"ocm-clusters-service",
					),
					Entry(
						"Accounts root",
						"/api/accounts_mgmt",
						"ocm-accounts-service",
					),
					Entry(
						"Accounts version",
						"/api/accounts_mgmt/v1",
						"ocm-accounts-service",
					),
					Entry(
						"Accounts collection",
						"/api/accounts_mgmt/v1/accounts",
						"ocm-accounts-service",
					),
					Entry(
						"Accounts item",
						"/api/accounts_mgmt/v1/accounts/123",
						"ocm-accounts-service",
					),
					Entry(
						"Logs root",
						"/api/service_logs",
						"ocm-logs-service",
					),
					Entry(
						"Logs version",
						"/api/service_logs/v1",
						"ocm-logs-service",
					),
					Entry(
						"Logs collection",
						"/api/service_logs/v1/accounts",
						"ocm-logs-service",
					),
					Entry(
						"Logs item",
						"/api/service_logs/v1/accounts/123",
						"ocm-logs-service",
					),
					Entry(
						"Future service not in existing service list",
						"/api/future_mgmt/v1/",
						"ocm-future_mgmt"),
				)
			})

			Describe("Request duration", func() {
				It("Honours subsystem", func() {
					// Prepare the handler:
					handler = interceptor.Wrap(RespondWith(http.StatusOK, nil))

					// Send the request:
					Send(http.MethodGet, "/api")

					// Verify the metrics:
					metrics := server.Metrics()
					Expect(metrics).To(MatchLine(`^my_request_duration_bucket\{.*\} .*$`))
					Expect(metrics).To(MatchLine(`^my_request_duration_sum\{.*\} .*$`))
					Expect(metrics).To(MatchLine(`^my_request_duration_count\{.*\} .*$`))
				})

				It("Honours buckets", func() {
					// Prepare the handler:
					handler = interceptor.Wrap(RespondWith(http.StatusOK, nil))

					// Send the request:
					Send(http.MethodGet, "/api")

					// Verify the metrics:
					metrics := server.Metrics()
					Expect(metrics).To(MatchLine(`^\w+_request_duration_bucket\{.*,le="0.1"\} .*$`))
					Expect(metrics).To(MatchLine(`^\w+_request_duration_bucket\{.*,le="1"\} .*$`))
					Expect(metrics).To(MatchLine(`^\w+_request_duration_bucket\{.*,le="10"\} .*$`))
					Expect(metrics).To(MatchLine(`^\w+_request_duration_bucket\{.*,le="30"\} .*$`))
					Expect(metrics).To(MatchLine(`^\w+_request_duration_bucket\{.*,le="\+Inf"\} .*$`))
				})

				DescribeTable(
					"Counts correctly",
					func(count int) {
						// Prepare the handler:
						handler = interceptor.Wrap(RespondWith(http.StatusOK, nil))

						// Send the requests:
						for i := 0; i < count; i++ {
							Send(http.MethodGet, "/api")
						}

						// Verify the metrics:
						metrics := server.Metrics()
						Expect(metrics).To(MatchLine(`^\w+_request_duration_count\{.*\} %d$`, count))
					},
					Entry("One", 1),
					Entry("Two", 2),
					Entry("Trhee", 3),
				)

				DescribeTable(
					"Includes method label",
					func(method string) {
						// Prepare the handler:
						handler = interceptor.Wrap(RespondWith(http.StatusOK, nil))

						// Send the requests:
						Send(method, "/api")

						// Verify the metrics:
						metrics := server.Metrics()
						Expect(metrics).To(MatchLine(`^\w+_request_duration_bucket\{.*method="%s".*\} .*$`, method))
						Expect(metrics).To(MatchLine(`^\w+_request_duration_sum\{.*method="%s".*\} .*$`, method))
						Expect(metrics).To(MatchLine(`^\w+_request_duration_count\{.*method="%s".*\} .*$`, method))
					},
					Entry("GET", http.MethodGet),
					Entry("POST", http.MethodPost),
					Entry("PATCH", http.MethodPatch),
					Entry("DELETE", http.MethodDelete),
				)

				DescribeTable(
					"Includes path label",
					func(path, label string) {
						// Prepare the handler:
						handler = interceptor.Wrap(RespondWith(http.StatusOK, nil))

						// Send the requests:
						Send(http.MethodGet, path)

						// Verify the metrics:
						metrics := server.Metrics()
						Expect(metrics).To(MatchLine(`^\w+_request_duration_bucket\{.*path="%s".*\} .*$`, label))
						Expect(metrics).To(MatchLine(`^\w+_request_duration_sum\{.*path="%s".*\} .*$`, label))
						Expect(metrics).To(MatchLine(`^\w+_request_duration_count\{.*path="%s".*\} .*$`, label))
					},
					Entry(
						"Empty",
						"",
						"/-",
					),
					Entry(
						"One slash",
						"/",
						"/-",
					),
					Entry(
						"Two slashes",
						"//",
						"/-",
					),
					Entry(
						"Tree slashes",
						"///",
						"/-",
					),
					Entry(
						"API root",
						"/api",
						"/api",
					),
					Entry(
						"API root with trailing slash",
						"/api/",
						"/api",
					),
					Entry(
						"Unknown root",
						"/junk/",
						"/-",
					),
					Entry(
						"Service root",
						"/api/clusters_mgmt",
						"/api/clusters_mgmt",
					),
					Entry(
						"Unknown service root",
						"/api/junk",
						"/-",
					),
					Entry(
						"Version root",
						"/api/clusters_mgmt/v1",
						"/api/clusters_mgmt/v1",
					),
					Entry(
						"Unknown version root",
						"/api/junk/v1",
						"/-",
					),
					Entry(
						"Collection",
						"/api/clusters_mgmt/v1/clusters",
						"/api/clusters_mgmt/v1/clusters",
					),
					Entry(
						"Unknown collection",
						"/api/clusters_mgmt/v1/junk",
						"/-",
					),
					Entry(
						"Collection item",
						"/api/clusters_mgmt/v1/clusters/123",
						"/api/clusters_mgmt/v1/clusters/-",
					),
					Entry(
						"Collection item action",
						"/api/clusters_mgmt/v1/clusters/123/hibernate",
						"/api/clusters_mgmt/v1/clusters/-/hibernate",
					),
					Entry(
						"Unknown collection item action",
						"/api/clusters_mgmt/v1/clusters/123/junk",
						"/-",
					),
					Entry(
						"Subcollection",
						"/api/clusters_mgmt/v1/clusters/123/groups",
						"/api/clusters_mgmt/v1/clusters/-/groups",
					),
					Entry(
						"Unknown subcollection",
						"/api/clusters_mgmt/v1/clusters/123/junks",
						"/-",
					),
					Entry(
						"Subcollection item",
						"/api/clusters_mgmt/v1/clusters/123/groups/456",
						"/api/clusters_mgmt/v1/clusters/-/groups/-",
					),
					Entry(
						"Too long",
						"/api/clusters_mgmt/v1/clusters/123/groups/456/junk",
						"/-",
					),
					Entry(
						"Explicitly specified path",
						"/my/path",
						"/my/path",
					),
					Entry(
						"Unknown path",
						"/your/path",
						"/-",
					),
				)

				DescribeTable(
					"Includes code label",
					func(code int) {
						// Prepare the handler:
						handler = interceptor.Wrap(RespondWith(code, nil))

						// Send the requests:
						Send(http.MethodGet, "/api")

						// Verify the metrics:
						metrics := server.Metrics()
						Expect(metrics).To(MatchLine(`^\w+_request_duration_bucket\{.*code="%d".*\} .*$`, code))
						Expect(metrics).To(MatchLine(`^\w+_request_duration_sum\{.*code="%d".*\} .*$`, code))
						Expect(metrics).To(MatchLine(`^\w+_request_duration_count\{.*code="%d".*\} .*$`, code))
					},
					Entry("200", http.StatusOK),
					Entry("201", http.StatusCreated),
					Entry("202", http.StatusAccepted),
					Entry("401", http.StatusUnauthorized),
					Entry("404", http.StatusNotFound),
					Entry("500", http.StatusInternalServerError),
				)

				DescribeTable(
					"Includes API service label",
					func(path, label string) {
						// Prepare the handler:
						handler = interceptor.Wrap(RespondWith(http.StatusOK, nil))

						// Send the requests:
						Send(http.MethodGet, path)

						// Verify the metrics:
						metrics := server.Metrics()
						Expect(metrics).To(MatchLine(`^\w+_request_duration_bucket\{.*apiservice="%s".*\} .*$`, label))
						Expect(metrics).To(MatchLine(`^\w+_request_duration_sum\{.*apiservice="%s".*\} .*$`, label))
						Expect(metrics).To(MatchLine(`^\w+_request_duration_count\{.*apiservice="%s".*\} .*$`, label))
					},
					Entry(
						"Empty",
						"",
						"",
					),
					Entry(
						"Root",
						"/",
						"",
					),
					Entry(
						"Clusters root",
						"/api/clusters_mgmt",
						"ocm-clusters-service",
					),
					Entry(
						"Clusters version",
						"/api/clusters_mgmt/v1",
						"ocm-clusters-service",
					),
					Entry(
						"Clusters collection",
						"/api/clusters_mgmt/v1/clusters",
						"ocm-clusters-service",
					),
					Entry(
						"Clusters item",
						"/api/clusters_mgmt/v1/clusters/123",
						"ocm-clusters-service",
					),
					Entry(
						"Accounts root",
						"/api/accounts_mgmt",
						"ocm-accounts-service",
					),
					Entry(
						"Accounts version",
						"/api/accounts_mgmt/v1",
						"ocm-accounts-service",
					),
					Entry(
						"Accounts collection",
						"/api/accounts_mgmt/v1/accounts",
						"ocm-accounts-service",
					),
					Entry(
						"Accounts item",
						"/api/accounts_mgmt/v1/accounts/123",
						"ocm-accounts-service",
					),
					Entry(
						"Logs root",
						"/api/service_logs",
						"ocm-logs-service",
					),
					Entry(
						"Logs version",
						"/api/service_logs/v1",
						"ocm-logs-service",
					),
					Entry(
						"Logs collection",
						"/api/service_logs/v1/accounts",
						"ocm-logs-service",
					),
					Entry(
						"Logs item",
						"/api/service_logs/v1/accounts/123",
						"ocm-logs-service",
					),
				)
			})
		*/
	})
})
