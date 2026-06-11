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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"os"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauthv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	publicv1 "github.com/osac-project/fulfillment-service/internal/api/osac/public/v1"
	"github.com/osac-project/fulfillment-service/internal/collections"
	. "github.com/osac-project/fulfillment-service/internal/testing"
)

var _ = Describe("GrpcExternalAuthInterceptor project metadata authorization", func() {
	var (
		ctx                   context.Context
		client                *grpc.ClientConn
		mock                  *GrpcExternalAuthMock
		interceptor           *GrpcExternalAuthInterceptor
		metadataFetcherCalled bool
		metadataFetcherID     string
	)

	BeforeEach(func() {
		var err error

		ctx = context.Background()
		mock = &GrpcExternalAuthMock{}

		crtFile, keyFile, caFile := LocalhostCertificateFiles()
		DeferCleanup(func() {
			_ = os.Remove(crtFile)
			_ = os.Remove(keyFile)
			_ = os.Remove(caFile)
		})
		caData, err := os.ReadFile(caFile)
		Expect(err).ToNot(HaveOccurred())

		caPool, err := x509.SystemCertPool()
		Expect(err).ToNot(HaveOccurred())
		ok := caPool.AppendCertsFromPEM(caData)
		Expect(ok).To(BeTrue())

		crt, err := tls.LoadX509KeyPair(crtFile, keyFile)
		Expect(err).ToNot(HaveOccurred())
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).ToNot(HaveOccurred())
		listener = tls.NewListener(listener, &tls.Config{
			RootCAs: caPool,
			Certificates: []tls.Certificate{
				crt,
			},
			NextProtos: []string{"h2"},
		})
		address := listener.Addr().String()

		endpoint := fmt.Sprintf("dns:///%s", address)
		creds := credentials.NewTLS(&tls.Config{
			RootCAs: caPool,
		})
		options := []grpc.DialOption{
			grpc.WithTransportCredentials(creds),
		}
		client, err = grpc.NewClient(endpoint, options...)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(client.Close)

		server := grpc.NewServer()
		DeferCleanup(server.Stop)
		envoyauthv3.RegisterAuthorizationServer(server, mock)
		go server.Serve(listener)

		metadataFetcherCalled = false
		metadataFetcherID = ""

		mockMetadataFetcher := func(ctx context.Context, id string) (string, string) {
			metadataFetcherCalled = true
			metadataFetcherID = id
			return "authoritative-tenant", "authoritative-project"
		}

		interceptor, err = NewGrpcExternalAuthInterceptor().
			SetLogger(logger).
			SetGrpcClient(client).
			SetMetadataFetcher(mockMetadataFetcher).
			Build()
		Expect(err).ToNot(HaveOccurred())
	})

	makeOkResponse := func(subject *Subject) *envoyauthv3.CheckResponse {
		header := ""
		if subject != nil {
			data, err := json.Marshal(subject)
			Expect(err).ToNot(HaveOccurred())
			header = string(data)
		}
		return &envoyauthv3.CheckResponse{
			Status: &status.Status{
				Code: int32(grpccodes.OK),
			},
			HttpResponse: &envoyauthv3.CheckResponse_OkResponse{
				OkResponse: &envoyauthv3.OkHttpResponse{
					Headers: []*envoycorev3.HeaderValueOption{{
						Header: &envoycorev3.HeaderValue{
							Key:   SubjectHeader,
							Value: header,
						},
					}},
				},
			},
		}
	}

	Describe("Projects Get operation", func() {
		It("Should fetch metadata from database and include in context extensions", func() {
			mock.Func = func(ctx context.Context,
				request *envoyauthv3.CheckRequest) (response *envoyauthv3.CheckResponse, err error) {
				Expect(request).ToNot(BeNil())
				extensions := request.GetAttributes().GetContextExtensions()
				Expect(extensions).To(HaveKeyWithValue("id", "project-123"))
				Expect(extensions).To(HaveKeyWithValue("tenant", "authoritative-tenant"))
				Expect(extensions).To(HaveKeyWithValue("name", "authoritative-project"))
				response = makeOkResponse(&Subject{
					User:    "my-user",
					Tenants: collections.NewSet("my-group"),
				})
				return
			}

			handler := func(context.Context, any) (any, error) {
				return nil, nil
			}
			info := &grpc.UnaryServerInfo{
				FullMethod: "/osac.public.v1.Projects/Get",
			}
			request := &publicv1.ProjectsGetRequest{
				Id: "project-123",
			}

			_, err := interceptor.UnaryServer(ctx, request, info, handler)

			Expect(err).ToNot(HaveOccurred())
			Expect(metadataFetcherCalled).To(BeTrue())
			Expect(metadataFetcherID).To(Equal("project-123"))
		})
	})

	Describe("Projects Delete operation", func() {
		It("Should fetch metadata from database and include in context extensions", func() {
			mock.Func = func(ctx context.Context,
				request *envoyauthv3.CheckRequest) (response *envoyauthv3.CheckResponse, err error) {
				Expect(request).ToNot(BeNil())
				extensions := request.GetAttributes().GetContextExtensions()
				Expect(extensions).To(HaveKeyWithValue("id", "project-456"))
				Expect(extensions).To(HaveKeyWithValue("tenant", "authoritative-tenant"))
				Expect(extensions).To(HaveKeyWithValue("name", "authoritative-project"))
				response = makeOkResponse(&Subject{
					User:    "my-user",
					Tenants: collections.NewSet("my-group"),
				})
				return
			}

			handler := func(context.Context, any) (any, error) {
				return nil, nil
			}
			info := &grpc.UnaryServerInfo{
				FullMethod: "/osac.public.v1.Projects/Delete",
			}
			request := &publicv1.ProjectsDeleteRequest{
				Id: "project-456",
			}

			_, err := interceptor.UnaryServer(ctx, request, info, handler)

			Expect(err).ToNot(HaveOccurred())
			Expect(metadataFetcherCalled).To(BeTrue())
			Expect(metadataFetcherID).To(Equal("project-456"))
		})
	})

	Describe("Projects Update operation", func() {
		It("Should fetch metadata from database and include in context extensions", func() {
			mock.Func = func(ctx context.Context,
				request *envoyauthv3.CheckRequest) (response *envoyauthv3.CheckResponse, err error) {
				Expect(request).ToNot(BeNil())
				extensions := request.GetAttributes().GetContextExtensions()
				Expect(extensions).To(HaveKeyWithValue("id", "project-789"))
				Expect(extensions).To(HaveKeyWithValue("tenant", "authoritative-tenant"))
				Expect(extensions).To(HaveKeyWithValue("name", "authoritative-project"))
				response = makeOkResponse(&Subject{
					User:    "my-user",
					Tenants: collections.NewSet("my-group"),
				})
				return
			}

			handler := func(context.Context, any) (any, error) {
				return nil, nil
			}
			info := &grpc.UnaryServerInfo{
				FullMethod: "/osac.public.v1.Projects/Update",
			}
			request := &publicv1.ProjectsUpdateRequest{
				Object: &publicv1.Project{
					Id: "project-789",
					Metadata: &publicv1.Metadata{
						Tenant: "client-spoofed-tenant",
						Name:   "client-spoofed-name",
					},
				},
				UpdateMask: &fieldmaskpb.FieldMask{},
			}

			_, err := interceptor.UnaryServer(ctx, request, info, handler)

			Expect(err).ToNot(HaveOccurred())
			Expect(metadataFetcherCalled).To(BeTrue())
			Expect(metadataFetcherID).To(Equal("project-789"))
		})

		It("Should ignore client-provided metadata values and use authoritative database values", func() {
			var capturedExtensions map[string]string

			mock.Func = func(ctx context.Context,
				request *envoyauthv3.CheckRequest) (response *envoyauthv3.CheckResponse, err error) {
				capturedExtensions = request.GetAttributes().GetContextExtensions()
				response = makeOkResponse(&Subject{
					User:    "my-user",
					Tenants: collections.NewSet("my-group"),
				})
				return
			}

			handler := func(context.Context, any) (any, error) {
				return nil, nil
			}
			info := &grpc.UnaryServerInfo{
				FullMethod: "/osac.public.v1.Projects/Update",
			}
			request := &publicv1.ProjectsUpdateRequest{
				Object: &publicv1.Project{
					Id: "project-999",
					Metadata: &publicv1.Metadata{
						Tenant: "malicious-tenant",
						Name:   "malicious-project",
					},
				},
				UpdateMask: &fieldmaskpb.FieldMask{},
			}

			_, err := interceptor.UnaryServer(ctx, request, info, handler)

			Expect(err).ToNot(HaveOccurred())
			Expect(capturedExtensions["tenant"]).To(Equal("authoritative-tenant"))
			Expect(capturedExtensions["name"]).To(Equal("authoritative-project"))
			Expect(capturedExtensions["tenant"]).ToNot(Equal("malicious-tenant"))
			Expect(capturedExtensions["name"]).ToNot(Equal("malicious-project"))
		})
	})

	Describe("Projects List operation", func() {
		It("Should not fetch metadata for list operations", func() {
			mock.Func = func(ctx context.Context,
				request *envoyauthv3.CheckRequest) (response *envoyauthv3.CheckResponse, err error) {
				Expect(request).ToNot(BeNil())
				extensions := request.GetAttributes().GetContextExtensions()
				Expect(extensions).ToNot(HaveKey("tenant"))
				Expect(extensions).ToNot(HaveKey("name"))
				response = makeOkResponse(&Subject{
					User:    "my-user",
					Tenants: collections.NewSet("my-group"),
				})
				return
			}

			handler := func(context.Context, any) (any, error) {
				return nil, nil
			}
			info := &grpc.UnaryServerInfo{
				FullMethod: "/osac.public.v1.Projects/List",
			}
			request := &publicv1.ProjectsListRequest{}

			_, err := interceptor.UnaryServer(ctx, request, info, handler)

			Expect(err).ToNot(HaveOccurred())
			Expect(metadataFetcherCalled).To(BeFalse())
		})
	})

	Describe("Other resource operations", func() {
		It("Should not fetch project metadata for Clusters operations", func() {
			mock.Func = func(ctx context.Context,
				request *envoyauthv3.CheckRequest) (response *envoyauthv3.CheckResponse, err error) {
				Expect(request).ToNot(BeNil())
				extensions := request.GetAttributes().GetContextExtensions()
				Expect(extensions).To(HaveKeyWithValue("id", "cluster-123"))
				Expect(extensions).ToNot(HaveKey("tenant"))
				Expect(extensions).ToNot(HaveKey("name"))
				response = makeOkResponse(&Subject{
					User: "my-user",
				})
				return
			}

			handler := func(context.Context, any) (any, error) {
				return nil, nil
			}
			info := &grpc.UnaryServerInfo{
				FullMethod: "/osac.public.v1.Clusters/Get",
			}
			request := &publicv1.ClustersGetRequest{
				Id: "cluster-123",
			}

			_, err := interceptor.UnaryServer(ctx, request, info, handler)

			Expect(err).ToNot(HaveOccurred())
			Expect(metadataFetcherCalled).To(BeFalse())
		})
	})
})
