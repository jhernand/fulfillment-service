/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package servers

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/osac-project/fulfillment-service/internal/auth"
	"github.com/osac-project/fulfillment-service/internal/collections"
)

var _ = Describe("addPublishedFilter", func() {
	var (
		server *ClusterCatalogItemsServer
		ctx    context.Context
	)

	BeforeEach(func() {
		server = &ClusterCatalogItemsServer{}
		ctx = auth.ContextWithSubject(context.Background(), &auth.Subject{
			User:    "test-admin",
			Tenants: collections.NewSet("my-tenant"),
		})
	})

	DescribeTable("composes filter correctly",
		func(input string, expected string) {
			result, err := server.addPublishedFilter(ctx, input)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(expected))
		},
		Entry("empty filter", "",
			"(this.published || this.metadata.creator == 'test-admin')"),
		Entry("simple filter", "this.id == '123'",
			"(this.id == '123') && (this.published || this.metadata.creator == 'test-admin')"),
		Entry("compound filter", "this.title == 'a' && this.template == 'b'",
			"(this.title == 'a' && this.template == 'b') && (this.published || this.metadata.creator == 'test-admin')"),
		Entry("valid filter with OR is safely composed", "true || true",
			"(true || true) && (this.published || this.metadata.creator == 'test-admin')"),
	)

	DescribeTable("rejects malformed filters",
		func(input string) {
			_, err := server.addPublishedFilter(ctx, input)
			Expect(err).To(HaveOccurred())
		},
		Entry("unbalanced parens to bypass published", `true) || (true`),
		Entry("unbalanced closing paren", `true)`),
		Entry("unbalanced opening paren", `(true`),
	)
})
