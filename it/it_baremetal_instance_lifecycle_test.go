/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package it

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
	publicv1 "github.com/osac-project/fulfillment-service/internal/api/osac/public/v1"
)

var _ = Describe("BareMetalInstance lifecycle", func() {
	var (
		bareMetalInstancesClient            publicv1.BareMetalInstancesClient
		privateBareMetalInstancesClient     privatev1.BareMetalInstancesClient
		bareMetalInstanceTemplatesClient    privatev1.BareMetalInstanceTemplatesClient
		bareMetalInstanceCatalogItemsClient privatev1.BareMetalInstanceCatalogItemsClient
		templateId                          string
		catalogItemId                       string
	)

	BeforeEach(func(ctx context.Context) {
		// Create clients
		bareMetalInstancesClient = publicv1.NewBareMetalInstancesClient(tool.ExternalView().UserConn())
		privateBareMetalInstancesClient = privatev1.NewBareMetalInstancesClient(tool.InternalView().AdminConn())
		bareMetalInstanceTemplatesClient = privatev1.NewBareMetalInstanceTemplatesClient(tool.InternalView().AdminConn())
		bareMetalInstanceCatalogItemsClient = privatev1.NewBareMetalInstanceCatalogItemsClient(tool.InternalView().AdminConn())

		// Create BareMetalInstanceTemplate
		templateResp, err := bareMetalInstanceTemplatesClient.Create(ctx, privatev1.BareMetalInstanceTemplatesCreateRequest_builder{
			Object: privatev1.BareMetalInstanceTemplate_builder{
				Title:       "Test BMI Template",
				Description: "Template for bare metal instance lifecycle test.",
			}.Build(),
		}.Build())
		Expect(err).ToNot(HaveOccurred())
		templateId = templateResp.GetObject().GetId()
		DeferCleanup(func(ctx context.Context) {
			_, err := bareMetalInstanceTemplatesClient.Delete(ctx, privatev1.BareMetalInstanceTemplatesDeleteRequest_builder{
				Id: templateId,
			}.Build())
			Expect(err).ToNot(HaveOccurred())
		})

		// Create BareMetalInstanceCatalogItem (must be published for public API access)
		catalogResp, err := bareMetalInstanceCatalogItemsClient.Create(ctx, privatev1.BareMetalInstanceCatalogItemsCreateRequest_builder{
			Object: privatev1.BareMetalInstanceCatalogItem_builder{
				Title:     "Test BMI Catalog Item",
				Template:  templateId,
				Published: true,
			}.Build(),
		}.Build())
		Expect(err).ToNot(HaveOccurred())
		catalogItemId = catalogResp.GetObject().GetId()
		DeferCleanup(func(ctx context.Context) {
			_, err := bareMetalInstanceCatalogItemsClient.Delete(ctx, privatev1.BareMetalInstanceCatalogItemsDeleteRequest_builder{
				Id: catalogItemId,
			}.Build())
			Expect(err).ToNot(HaveOccurred())
		})
	})

	It("Creates a BareMetalInstance and verifies fields", func(ctx context.Context) {
		// Create BareMetalInstance via public API
		createResp, err := bareMetalInstancesClient.Create(ctx, publicv1.BareMetalInstancesCreateRequest_builder{
			Object: publicv1.BareMetalInstance_builder{
				Spec: publicv1.BareMetalInstanceSpec_builder{
					CatalogItem: catalogItemId,
				}.Build(),
			}.Build(),
		}.Build())
		Expect(err).ToNot(HaveOccurred())
		object := createResp.GetObject()
		Expect(object).ToNot(BeNil())
		bareMetalInstanceId := object.GetId()
		DeferCleanup(func(ctx context.Context) {
			_, err := privateBareMetalInstancesClient.Delete(ctx, privatev1.BareMetalInstancesDeleteRequest_builder{
				Id: bareMetalInstanceId,
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Eventually(func(g Gomega) {
				_, err := privateBareMetalInstancesClient.Get(ctx, privatev1.BareMetalInstancesGetRequest_builder{
					Id: bareMetalInstanceId,
				}.Build())
				g.Expect(err).To(HaveOccurred())
				status, ok := grpcstatus.FromError(err)
				g.Expect(ok).To(BeTrue())
				g.Expect(status.Code()).To(Equal(grpccodes.NotFound))
			}, time.Minute, time.Second).Should(Succeed())
		})

		// Set BareMetalInstance to RUNNING state via private Update API
		bmiGetResp, err := privateBareMetalInstancesClient.Get(ctx, privatev1.BareMetalInstancesGetRequest_builder{
			Id: bareMetalInstanceId,
		}.Build())
		Expect(err).ToNot(HaveOccurred())
		bmi := bmiGetResp.GetObject()
		bmi.SetStatus(privatev1.BareMetalInstanceStatus_builder{
			State: privatev1.BareMetalInstanceState_BARE_METAL_INSTANCE_STATE_RUNNING,
		}.Build())
		_, err = privateBareMetalInstancesClient.Update(ctx, privatev1.BareMetalInstancesUpdateRequest_builder{
			Object:     bmi,
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"status.state"}},
		}.Build())
		Expect(err).ToNot(HaveOccurred())

		// Verify the BareMetalInstance fields via public Get
		getResp, err := bareMetalInstancesClient.Get(ctx, publicv1.BareMetalInstancesGetRequest_builder{
			Id: bareMetalInstanceId,
		}.Build())
		Expect(err).ToNot(HaveOccurred())
		object = getResp.GetObject()
		metadata := object.GetMetadata()
		Expect(metadata).ToNot(BeNil())
		Expect(metadata.HasCreationTimestamp()).To(BeTrue())
		Expect(metadata.HasDeletionTimestamp()).To(BeFalse())
		Expect(object.GetSpec().GetCatalogItem()).To(Equal(catalogItemId),
			"BareMetalInstance should persist catalog item reference")
		Expect(object.GetStatus().GetState()).To(
			Equal(publicv1.BareMetalInstanceState_BARE_METAL_INSTANCE_STATE_RUNNING),
			"BareMetalInstance should be in RUNNING state after status override")
	})

	It("Rejects BareMetalInstance with non-existent catalog item", func(ctx context.Context) {
		_, err := bareMetalInstancesClient.Create(ctx, publicv1.BareMetalInstancesCreateRequest_builder{
			Object: publicv1.BareMetalInstance_builder{
				Spec: publicv1.BareMetalInstanceSpec_builder{
					CatalogItem: "non-existent-catalog-item",
				}.Build(),
			}.Build(),
		}.Build())
		Expect(err).To(HaveOccurred())
		status, ok := grpcstatus.FromError(err)
		Expect(ok).To(BeTrue())
		Expect(status.Code()).To(Equal(grpccodes.NotFound))
	})
})
