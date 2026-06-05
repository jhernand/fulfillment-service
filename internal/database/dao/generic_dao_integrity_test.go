/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package dao

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
)

var _ = Describe("Referential integrity", func() {
	var generic *GenericDAO[*privatev1.Cluster]

	BeforeEach(func() {
		var err error

		// Create the DAO:
		generic, err = NewGenericDAO[*privatev1.Cluster]().
			SetLogger(logger).
			Build()
		Expect(err).ToNot(HaveOccurred())
	})

	It("Returns 'reference' error when creating object with non-existent tenant", func() {
		_, err := generic.Create().
			SetObject(&privatev1.Cluster{
				Metadata: privatev1.Metadata_builder{
					Tenant: "my-tenant",
				}.Build(),
			}).
			Do(ctx)
		Expect(err).To(HaveOccurred())
		var referenceErr *ErrReference
		Expect(errors.As(err, &referenceErr)).To(BeTrue())
		Expect(referenceErr.Reason).To(Equal("tenant 'my-tenant' doesn't exist"))
	})
})
