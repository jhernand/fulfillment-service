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
	"crypto/x509"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"

	kubefiles "github.com/osac-project/fulfillment-service/internal/kubernetes/files"
	. "github.com/osac-project/fulfillment-service/internal/testing"
)

var _ = Describe("JSON web key set authentication interceptor", func() {
	Describe("Building", func() {
		var caPool *x509.CertPool

		BeforeEach(func() {
			caPool = x509.NewCertPool()
		})

		It("Rejects builder without logger", func() {
			_, err := NewGrpcAuthnInterceptor().
				AddIssuer("https://my-issuer.example.com").
				SetCaPool(caPool).
				Build()
			Expect(err).To(MatchError("logger is mandatory"))
		})

		It("Can't be built without at least one issuer", func() {
			_, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				SetCaPool(caPool).
				Build()
			Expect(err).To(MatchError("at least one issuer must be configured"))
		})

		It("Can't be built without a CA pool", func() {
			_, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer("https://my-issuer.example.com").
				Build()
			Expect(err).To(MatchError("CA pool is mandatory"))
		})

		It("Can't be built with an empty issuer", func() {
			_, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer("").
				SetCaPool(caPool).
				Build()
			Expect(err).To(MatchError("invalid empty issuer URL"))
		})

		It("Can't be built with an invalid issuer URL", func() {
			_, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer("://bad").
				SetCaPool(caPool).
				Build()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid issuer URL"))
		})

		It("Can't be built with an HTTP issuer URL", func() {
			_, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer("http://my-issuer.example.com").
				SetCaPool(caPool).
				Build()
			Expect(err).To(MatchError(
				"issuer URL 'http://my-issuer.example.com' must use the HTTPS scheme",
			))
		})

		It("Can be built with valid parameters", func() {
			_, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer("https://my-issuer.example.com").
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
		})

		It("Can't be built if the Kubernetes issuer is enabled but the token file does not exist", func() {
			// Create a temporary and empty root directory, so the check for the token file will fail
			// because it doesn't exist.
			tmpDir, err := os.MkdirTemp("", "*.test")
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(func() {
				err := os.RemoveAll(tmpDir)
				Expect(err).ToNot(HaveOccurred())
			})

			// Create an interceptor that automatically adds the Kubernetes issuer, but does not have any
			// other issuer configured. This will make the build fail because there will be no issuers.
			_, err = NewGrpcAuthnInterceptor().
				SetLogger(logger).
				SetRoot(tmpDir).
				SetCaPool(caPool).
				AddKubernetesIssuer(true).
				Build()
			Expect(err).To(MatchError("at least one issuer must be configured"))
		})

		It("Cannot be built with a negative keys TTL", func() {
			_, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer("https://my-issuer.example.com").
				SetCaPool(caPool).
				SetKeysTTL(-time.Second).
				Build()
			Expect(err).To(MatchError("keys TTL must be zero or positive"))
		})
	})

	Describe("Token validation", func() {
		var (
			tmpDir    string
			issuerUrl string
			caPool    *x509.CertPool
		)

		BeforeEach(func() {
			var err error

			// Create the test server:
			issuerServer := ghttp.NewUnstartedServer()
			issuerServer.Writer = GinkgoWriter
			issuerServer.HTTPTestServer.EnableHTTP2 = true
			issuerServer.HTTPTestServer.StartTLS()
			DeferCleanup(issuerServer.Close)
			issuerUrl = issuerServer.URL()

			// Serve the JSON web key set using the shared test keys:
			issuerServer.RouteToHandler(
				http.MethodGet,
				"/.well-known/jwks.json",
				ghttp.RespondWithJSONEncoded(http.StatusOK, MakeJwksObject()),
			)

			// Serve the OIDC discovery document:
			issuerServer.RouteToHandler(
				http.MethodGet,
				"/.well-known/openid-configuration",
				ghttp.RespondWithJSONEncoded(
					http.StatusOK,
					map[string]any{
						"issuer":   issuerServer.URL(),
						"jwks_uri": issuerServer.URL() + "/.well-known/jwks.json",
					},
				),
			)

			// Build a CA pool that trusts the test server's certificate:
			caPool = x509.NewCertPool()
			caPool.AddCert(issuerServer.HTTPTestServer.Certificate())

			// Create a temporary directory where the tests will create the emulated Kubernetes service
			// account token file:
			tmpDir, err = os.MkdirTemp("", "*.test")
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(func() {
				err := os.RemoveAll(tmpDir)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		It("Rejects request without authorization header for private method", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(Equal("method '/my_package/MyMethod' requires authentication"))
		})

		It("Rejects bad authorization type", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenString(issuerUrl, "Bearer", time.Minute)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bad "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(Equal("authentication scheme 'Bad' is not supported"))
		})

		It("Rejects bad bearer token", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(
				Authorization, "Bearer junk",
			))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(Equal("bearer token is malformed"))
		})

		It("Rejects expired bearer token", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenString(issuerUrl, "Bearer", -time.Minute)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(Equal("bearer token is expired"))
		})

		It("Accepts token without 'typ' claim", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenObject(jwt.MapClaims{
				"iss": issuerUrl,
				"typ": nil,
			}).Raw
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Rejects 'typ' claim with incorrect type", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenString(issuerUrl, "Junk", time.Minute)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(Equal("bearer token type 'Junk' is not allowed"))
		})

		It("Rejects refresh tokens", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenString(issuerUrl, "Refresh", time.Minute)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(Equal("bearer token type 'Refresh' is not allowed"))
		})

		It("Rejects offline tokens", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenString(issuerUrl, "Offline", time.Minute)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(Equal("bearer token type 'Offline' is not allowed"))
		})

		It("Rejects token issued in the future", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			iat := time.Now().Add(1 * time.Minute)
			exp := iat.Add(1 * time.Minute)
			token := MakeTokenObject(jwt.MapClaims{
				"iss": issuerUrl,
				"iat": iat.Unix(),
				"exp": exp.Unix(),
			}).Raw
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(Equal("bearer token was issued in the future"))
		})

		It("Rejects token that isn't valid yet", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			iat := time.Now()
			nbf := iat.Add(1 * time.Minute)
			exp := nbf.Add(1 * time.Minute)
			token := MakeTokenObject(jwt.MapClaims{
				"iss": issuerUrl,
				"iat": iat.Unix(),
				"nbf": nbf.Unix(),
				"exp": exp.Unix(),
			}).Raw
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(Equal("bearer token is not valid yet"))
		})

		It("Rejects token without 'sub' claim", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenObject(jwt.MapClaims{
				"iss": issuerUrl,
				"sub": nil,
			}).Raw
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(Equal("bearer token does not contain required claim 'sub'"))
		})

		It("Rejects tokens with untrusted issuer", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenString(
				"https://untrusted-issuer.example.com",
				"Bearer",
				time.Minute,
			)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(Equal(
				"token issuer 'https://untrusted-issuer.example.com' is not trusted",
			))
		})

		It("Stores the validated token in the context", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenObject(jwt.MapClaims{
				"iss":                issuerUrl,
				"sub":                "user-123",
				"preferred_username": "my_user",
			}).Raw
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					stored := TokenFromContext(ctx)
					Expect(stored).ToNot(BeNil())
					Expect(stored.Valid).To(BeTrue())
					Expect(stored.Raw).To(Equal(token))
					return nil, nil
				},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Accepts token expired within the configured tolerance", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				SetTolerance(5 * time.Second).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenObject(jwt.MapClaims{
				"iss": issuerUrl,
				"exp": time.Now().Add(-time.Second).Unix(),
			}).Raw
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Rejects token expired outside the configured tolerance", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				SetTolerance(5 * time.Second).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenObject(jwt.MapClaims{
				"iss": issuerUrl,
				"exp": time.Now().Add(-time.Minute).Unix(),
			}).Raw
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(
				Authorization, "Bearer "+token,
			))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(Equal("bearer token is expired"))
		})

		It("Automatically adds the Kubernetes issuer", func(ctx context.Context) {
			// Create the token:
			token := MakeTokenString(issuerUrl, "Bearer", time.Minute)

			// Create a temporary file that simulates the Kubernetes service account token file, but using
			// our own token issuer URL. The interceptor will load this token file and automatically add the
			// issuer URL.
			tokenFile := filepath.Join(tmpDir, kubefiles.ServiceAccountToken)
			tokenDir := filepath.Dir(tokenFile)
			err := os.MkdirAll(tokenDir, 0700)
			Expect(err).ToNot(HaveOccurred())
			err = os.WriteFile(tokenFile, []byte(token), 0600)
			Expect(err).ToNot(HaveOccurred())

			// Create an interceptor that automatically adds the Kubernetes issuer:
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				SetRoot(tmpDir).
				SetCaPool(caPool).
				AddKubernetesIssuer(true).
				Build()
			Expect(err).ToNot(HaveOccurred())

			// Send a request to the interceptor:
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			handled := false
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					stored := TokenFromContext(ctx)
					Expect(stored).ToNot(BeNil())
					Expect(stored.Valid).To(BeTrue())
					Expect(stored.Raw).To(Equal(token))
					handled = true
					return nil, nil
				},
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(handled).To(BeTrue())
		})

		It("Doesn't require authorization header for anoymous method", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				AddAnonymousMethodRegex(`^/my_package/.*$`).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Accepts malformed authorization header for anoymous method", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				AddAnonymousMethodRegex(`^/my_package/.*$`).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenString(issuerUrl, "Bearer", time.Minute)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Ignores expired token for anoymous method", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				AddAnonymousMethodRegex(`^/my_package/.*$`).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenObject(jwt.MapClaims{
				"exp": time.Now().Add(-1 * time.Minute).Unix(),
			}).Raw
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Stores valid token even for anonymous method", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				AddAnonymousMethodRegex(`^/my_package/.*$`).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			token := MakeTokenString(issuerUrl, "Bearer", time.Minute)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(Authorization, "Bearer "+token))
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					stored := TokenFromContext(ctx)
					Expect(stored).ToNot(BeNil())
					Expect(stored.Valid).To(BeTrue())
					Expect(stored.Raw).To(Equal(token))
					return nil, nil
				},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Combines multiple anoymous method patterns", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				AddAnonymousMethodRegex(`^/my_package/.*$`).
				AddAnonymousMethodRegex(`^/your_package/.*$`).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).ToNot(HaveOccurred())
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/your_package/YourMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Rejects multiple authorization headers", func(ctx context.Context) {
			interceptor, err := NewGrpcAuthnInterceptor().
				SetLogger(logger).
				AddIssuer(issuerUrl).
				SetCaPool(caPool).
				Build()
			Expect(err).ToNot(HaveOccurred())
			md := metadata.Pairs(
				Authorization, "Bearer token1",
				Authorization, "Bearer token2",
			)
			ctx = metadata.NewIncomingContext(ctx, md)
			_, err = interceptor.UnaryServer(
				ctx,
				nil,
				&grpc.UnaryServerInfo{
					FullMethod: "/my_package/MyMethod",
				},
				func(ctx context.Context, req any) (any, error) {
					return nil, nil
				},
			)
			Expect(err).To(HaveOccurred())
			status, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(status.Code()).To(Equal(grpccodes.Unauthenticated))
			Expect(status.Message()).To(ContainSubstring("at most one"))
		})
	})
})
