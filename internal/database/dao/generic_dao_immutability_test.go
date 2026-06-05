/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package dao

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
)

var _ = Describe("Immutable fields", func() {
	var generic *GenericDAO[*privatev1.Organization]

	BeforeEach(func() {
		var err error

		// Create the DAO:
		generic, err = NewGenericDAO[*privatev1.Organization]().
			SetLogger(logger).
			Build()
		Expect(err).ToNot(HaveOccurred())

		// Create a test object:
		_, err = generic.Create().
			SetObject(privatev1.Organization_builder{
				Id: "my-tenant",
				Metadata: privatev1.Metadata_builder{
					Name:   "my-tenant",
					Tenant: "my-tenant",
				}.Build(),
			}.Build()).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())
	})

	It("Rejects update that changes one immutable field", func() {
		_, err := generic.Update().
			SetObject(privatev1.Organization_builder{
				Id: "my-tenant",
				Metadata: privatev1.Metadata_builder{
					Name:   "your-name",
					Tenant: "my-tenant",
				}.Build(),
			}.Build()).
			Do(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(&ErrImmutable{
			Fields: []string{
				"metadata.name",
			},
		}))
	})

	It("Rejects update that changes two immutable field", func() {
		_, err := generic.Update().
			SetObject(privatev1.Organization_builder{
				Id: "my-tenant",
				Metadata: privatev1.Metadata_builder{
					Name:   "your-name",
					Tenant: "your-tenant",
				}.Build(),
			}.Build()).
			Do(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(&ErrImmutable{
			Fields: []string{
				"metadata.name",
				"metadata.tenant",
			},
		}))
	})

	It("Allows update that includes but doesn't change an immutable field", func() {
		_, err := generic.Update().
			SetObject(privatev1.Organization_builder{
				Id: "my-tenant",
				Metadata: privatev1.Metadata_builder{
					Name:   "my-tenant",
					Tenant: "my-tenant",
				}.Build(),
			}.Build()).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())
	})

	It("Allows update of other fields", func() {
		_, err := generic.Update().
			SetObject(privatev1.Organization_builder{
				Id: "my-tenant",
				Metadata: privatev1.Metadata_builder{
					Name:   "my-tenant",
					Tenant: "my-tenant",
					Labels: map[string]string{
						"my-label": "my-value",
					},
				}.Build(),
			}.Build()).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())
	})
})
