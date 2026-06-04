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
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
	"github.com/osac-project/fulfillment-service/internal/controllers/finalizers"
	"github.com/osac-project/fulfillment-service/internal/uuid"
)

func createOrg(ctx context.Context, client privatev1.OrganizationsClient, name string) string {
	createResponse, err := client.Create(ctx, privatev1.OrganizationsCreateRequest_builder{
		Object: privatev1.Organization_builder{
			Metadata: privatev1.Metadata_builder{
				Name: name,
			}.Build(),
		}.Build(),
	}.Build())
	Expect(err).ToNot(HaveOccurred())
	id := createResponse.GetObject().GetId()
	DeferCleanup(func() {
		_, _ = client.Delete(ctx, privatev1.OrganizationsDeleteRequest_builder{
			Id: id,
		}.Build())
	})
	return id
}

func waitForOrgSynced(ctx context.Context, client privatev1.OrganizationsClient, id string) {
	Eventually(
		func(g Gomega) {
			getResponse, err := client.Get(ctx, privatev1.OrganizationsGetRequest_builder{
				Id: id,
			}.Build())
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(getResponse.GetObject().GetStatus().GetState()).To(
				Equal(privatev1.OrganizationState_ORGANIZATION_STATE_SYNCED),
			)
			g.Expect(getResponse.GetObject().GetStatus().GetBreakGlassUserId()).ToNot(BeEmpty())
		},
		time.Minute,
		time.Second,
	).Should(Succeed())
}

func verifyOrgInKeycloak(ctx context.Context, name string) {
	code, body, err := tool.KeycloakAdminRequest(ctx, http.MethodGet,
		fmt.Sprintf("/organizations?search=%s", name), nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(code).To(Equal(http.StatusOK))
	var kcOrgs []map[string]any
	Expect(json.Unmarshal(body, &kcOrgs)).To(Succeed())
	Expect(kcOrgs).ToNot(BeEmpty())
}

func verifyOrgRemovedFromKeycloak(ctx context.Context, name string) {
	Eventually(
		func(g Gomega) {
			code, body, err := tool.KeycloakAdminRequest(ctx, http.MethodGet,
				fmt.Sprintf("/organizations?search=%s", name), nil)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(code).To(Equal(http.StatusOK))
			var kcOrgs []map[string]any
			g.Expect(json.Unmarshal(body, &kcOrgs)).To(Succeed())
			g.Expect(kcOrgs).To(BeEmpty())
		},
		time.Minute,
		time.Second,
	).Should(Succeed())
}

var _ = Describe("Organization lifecycle", func() {
	var (
		ctx    context.Context
		client privatev1.OrganizationsClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		client = privatev1.NewOrganizationsClient(tool.InternalView().AdminConn())
	})

	It("CRUD happy flow", func() {
		name := fmt.Sprintf("test-%s", uuid.New())
		id := createOrg(ctx, client, name)
		waitForOrgSynced(ctx, client, id)

		getResponse, err := client.Get(ctx, privatev1.OrganizationsGetRequest_builder{
			Id: id,
		}.Build())
		Expect(err).ToNot(HaveOccurred())
		object := getResponse.GetObject()
		Expect(object.GetMetadata().GetName()).To(Equal(name))
		Expect(object.GetStatus().GetBreakGlassUserId()).ToNot(BeEmpty())
		Expect(object.GetStatus().GetIdpOrganizationName()).To(Equal(name))

		listResponse, err := client.List(ctx, privatev1.OrganizationsListRequest_builder{}.Build())
		Expect(err).ToNot(HaveOccurred())
		ids := make([]string, len(listResponse.GetItems()))
		for i, item := range listResponse.GetItems() {
			ids[i] = item.GetId()
		}
		Expect(ids).To(ContainElement(id))

		verifyOrgInKeycloak(ctx, name)

		_, err = client.Delete(ctx, privatev1.OrganizationsDeleteRequest_builder{
			Id: id,
		}.Build())
		Expect(err).ToNot(HaveOccurred())

		Eventually(
			func(g Gomega) {
				_, err := client.Get(ctx, privatev1.OrganizationsGetRequest_builder{
					Id: id,
				}.Build())
				g.Expect(err).To(HaveOccurred())
				status, ok := grpcstatus.FromError(err)
				g.Expect(ok).To(BeTrue())
				g.Expect(status.Code()).To(Equal(grpccodes.NotFound))
			},
			time.Minute,
			time.Second,
		).Should(Succeed())

		verifyOrgRemovedFromKeycloak(ctx, name)
	})

	It("Break-glass auth and RBAC", func() {
		name := fmt.Sprintf("test-%s", uuid.New())
		id := createOrg(ctx, client, name)

		var breakGlassUserId string
		var breakGlassUsername string
		var breakGlassPassword string
		Eventually(
			func(g Gomega) {
				getResponse, err := client.Get(ctx, privatev1.OrganizationsGetRequest_builder{
					Id: id,
				}.Build())
				g.Expect(err).ToNot(HaveOccurred())
				status := getResponse.GetObject().GetStatus()
				g.Expect(status.GetState()).To(
					Equal(privatev1.OrganizationState_ORGANIZATION_STATE_SYNCED),
				)
				g.Expect(status.GetBreakGlassUserId()).ToNot(BeEmpty())
				breakGlassUserId = status.GetBreakGlassUserId()
				creds := status.GetBreakGlassCredentials()
				if creds != nil {
					breakGlassUsername = creds.GetUsername()
					breakGlassPassword = creds.GetPassword()
				}
			},
			time.Minute,
			time.Second,
		).Should(Succeed())

		if breakGlassPassword == "" {
			breakGlassPassword = "test-break-glass-pass-123!"
			breakGlassUsername = fmt.Sprintf("%s-osac-break-glass", name)
		}

		// Set a known password and clear the UPDATE_PASSWORD required action so
		// the password grant flow works without interactive consent:
		code, _, err := tool.KeycloakAdminRequest(ctx, http.MethodPut,
			fmt.Sprintf("/users/%s/reset-password", breakGlassUserId),
			map[string]any{
				"type":      "password",
				"value":     breakGlassPassword,
				"temporary": false,
			})
		Expect(err).ToNot(HaveOccurred())
		Expect(code).To(Equal(http.StatusNoContent))

		code, _, err = tool.KeycloakAdminRequest(ctx, http.MethodPut,
			fmt.Sprintf("/users/%s", breakGlassUserId),
			map[string]any{
				"requiredActions": []string{},
			})
		Expect(err).ToNot(HaveOccurred())
		Expect(code).To(Equal(http.StatusNoContent))

		tokenSource, err := tool.makeKeycloakTokenSource(ctx, breakGlassUsername, breakGlassPassword)
		Expect(err).ToNot(HaveOccurred())

		bgInternalConn, err := tool.makeGrpcConn(internalServiceAddr, tokenSource)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			_ = bgInternalConn.Close()
		})

		bgPrivateClient := privatev1.NewOrganizationsClient(bgInternalConn)
		_, err = bgPrivateClient.List(ctx, privatev1.OrganizationsListRequest_builder{}.Build())
		Expect(err).To(HaveOccurred())
		status, ok := grpcstatus.FromError(err)
		Expect(ok).To(BeTrue())
		Expect(status.Code()).To(Equal(grpccodes.PermissionDenied))
	})

	It("Duplicate org name fails", func() {
		name := fmt.Sprintf("test-%s", uuid.New())
		id := createOrg(ctx, client, name)
		waitForOrgSynced(ctx, client, id)

		_, err := client.Create(ctx, privatev1.OrganizationsCreateRequest_builder{
			Object: privatev1.Organization_builder{
				Metadata: privatev1.Metadata_builder{
					Name: name,
				}.Build(),
			}.Build(),
		}.Build())
		Expect(err).To(HaveOccurred())
		status, ok := grpcstatus.FromError(err)
		Expect(ok).To(BeTrue())
		Expect(status.Code()).To(Equal(grpccodes.AlreadyExists))
	})

	It("Rename org is rejected", func() {
		name := fmt.Sprintf("test-%s", uuid.New())
		id := createOrg(ctx, client, name)

		Eventually(
			func(g Gomega) {
				getResponse, err := client.Get(ctx, privatev1.OrganizationsGetRequest_builder{
					Id: id,
				}.Build())
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(getResponse.GetObject().GetMetadata().GetFinalizers()).To(
					ContainElement(finalizers.Controller),
				)
				g.Expect(getResponse.GetObject().GetStatus().GetState()).To(
					Equal(privatev1.OrganizationState_ORGANIZATION_STATE_SYNCED),
				)
			},
			time.Minute,
			time.Second,
		).Should(Succeed())

		newName := fmt.Sprintf("renamed-%s", uuid.New())
		_, err := client.Update(ctx, privatev1.OrganizationsUpdateRequest_builder{
			Object: privatev1.Organization_builder{
				Id: id,
				Metadata: privatev1.Metadata_builder{
					Name: newName,
				}.Build(),
			}.Build(),
		}.Build())
		Expect(err).To(HaveOccurred())
		status, ok := grpcstatus.FromError(err)
		Expect(ok).To(BeTrue())
		Expect(status.Code()).To(Equal(grpccodes.InvalidArgument))
	})
})
