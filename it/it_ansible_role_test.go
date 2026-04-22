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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
	publicv1 "github.com/osac-project/fulfillment-service/internal/api/osac/public/v1"
	"github.com/osac-project/fulfillment-service/internal/kubernetes/gvks"
	"github.com/osac-project/fulfillment-service/internal/kubernetes/labels"
	"github.com/osac-project/fulfillment-service/internal/uuid"
)

var _ = Describe("Ansible role in templates", func() {
	Describe("Cluster templates", func() {
		var (
			ctx             context.Context
			clustersClient  publicv1.ClustersClient
			templatesClient privatev1.ClusterTemplatesClient
			hostTypeId      string
		)

		BeforeEach(func() {
			// Create a context:
			ctx = context.Background()

			// Create the clients:
			clustersClient = publicv1.NewClustersClient(tool.UserConn())
			templatesClient = privatev1.NewClusterTemplatesClient(tool.AdminConn())

			// Create a host type for use in the test template:
			hostTypesClient := privatev1.NewHostTypesClient(tool.AdminConn())
			hostTypeId = fmt.Sprintf("my_host_type_%s", uuid.New())
			_, err := hostTypesClient.Create(ctx, privatev1.HostTypesCreateRequest_builder{
				Object: privatev1.HostType_builder{
					Id: hostTypeId,
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
		})

		It("Uses 'ansible_role' as template identifier in the cluster when set", func() {
			// Create a template with an 'ansible_role':
			templateId := fmt.Sprintf("my_template_%s", uuid.New())
			_, err := templatesClient.Create(ctx, privatev1.ClusterTemplatesCreateRequest_builder{
				Object: privatev1.ClusterTemplate_builder{
					Id:          templateId,
					Title:       "My template",
					Description: "My template.",
					AnsibleRole: "my_ansible_role",
					NodeSets: map[string]*privatev1.ClusterTemplateNodeSet{
						"my_node_set": privatev1.ClusterTemplateNodeSet_builder{
							HostType: hostTypeId,
							Size:     1,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(func() {
				_, err := templatesClient.Delete(ctx, privatev1.ClusterTemplatesDeleteRequest_builder{
					Id: templateId,
				}.Build())
				Expect(err).ToNot(HaveOccurred())
			})

			// Create a cluster using that template:
			response, err := clustersClient.Create(ctx, publicv1.ClustersCreateRequest_builder{
				Object: publicv1.Cluster_builder{
					Spec: publicv1.ClusterSpec_builder{
						Template: templateId,
					}.Build(),
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			cluster := response.GetObject()
			DeferCleanup(func() {
				_, err := clustersClient.Delete(ctx, publicv1.ClustersDeleteRequest_builder{
					Id: cluster.GetId(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
			})

			// Check that the Kubernetes cluster uses the Ansible role as the template identifier:
			kubeClient := tool.KubeClient()
			clusterList := &unstructured.UnstructuredList{}
			clusterList.SetGroupVersionKind(gvks.ClusterOrderList)
			Eventually(
				func(g Gomega) {
					err := kubeClient.List(ctx, clusterList, crclient.MatchingLabels{
						labels.ClusterOrderUuid: cluster.GetId(),
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(clusterList.Items).To(HaveLen(1))
					resultId, found, err := unstructured.NestedString(
						clusterList.Items[0].Object, "spec", "templateID",
					)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(found).To(BeTrue())
					g.Expect(resultId).To(Equal("my_ansible_role"))
				},
				time.Minute,
				time.Second,
			).Should(Succeed())
		})

		It("Uses the template identifier directly when 'ansible_role' is not set", func() {
			// Create a template without an 'ansible_role':
			templateId := fmt.Sprintf("my_template_%s", uuid.New())
			_, err := templatesClient.Create(ctx, privatev1.ClusterTemplatesCreateRequest_builder{
				Object: privatev1.ClusterTemplate_builder{
					Id:          templateId,
					Title:       "My template",
					Description: "My template.",
					NodeSets: map[string]*privatev1.ClusterTemplateNodeSet{
						"my_node_set": privatev1.ClusterTemplateNodeSet_builder{
							HostType: hostTypeId,
							Size:     1,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(func() {
				_, err := templatesClient.Delete(ctx, privatev1.ClusterTemplatesDeleteRequest_builder{
					Id: templateId,
				}.Build())
				Expect(err).ToNot(HaveOccurred())
			})

			// Create a cluster using that template:
			response, err := clustersClient.Create(ctx, publicv1.ClustersCreateRequest_builder{
				Object: publicv1.Cluster_builder{
					Spec: publicv1.ClusterSpec_builder{
						Template: templateId,
					}.Build(),
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			cluster := response.GetObject()
			DeferCleanup(func() {
				_, err := clustersClient.Delete(ctx, publicv1.ClustersDeleteRequest_builder{
					Id: cluster.GetId(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
			})

			// Check that the Kubernetes cluster uses the template identifier:
			kubeClient := tool.KubeClient()
			clusterList := &unstructured.UnstructuredList{}
			clusterList.SetGroupVersionKind(gvks.ClusterOrderList)
			Eventually(
				func(g Gomega) {
					err := kubeClient.List(ctx, clusterList, crclient.MatchingLabels{
						labels.ClusterOrderUuid: cluster.GetId(),
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(clusterList.Items).To(HaveLen(1))
					resultId, found, err := unstructured.NestedString(
						clusterList.Items[0].Object, "spec", "templateID",
					)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(found).To(BeTrue())
					g.Expect(resultId).To(Equal(templateId))
				},
				time.Minute,
				time.Second,
			).Should(Succeed())
		})
	})

	Describe("Compute instance templates", func() {
		var (
			ctx             context.Context
			instancesClient publicv1.ComputeInstancesClient
			templatesClient privatev1.ComputeInstanceTemplatesClient
		)

		BeforeEach(func() {
			ctx = context.Background()
			instancesClient = publicv1.NewComputeInstancesClient(tool.UserConn())
			templatesClient = privatev1.NewComputeInstanceTemplatesClient(tool.AdminConn())
		})

		It("Uses 'ansible_role' as template identifier in the compute instance when set", func() {
			// Create a template with an 'ansible_role':
			templateId := fmt.Sprintf("my_template_%s", uuid.New())
			_, err := templatesClient.Create(
				ctx, privatev1.ComputeInstanceTemplatesCreateRequest_builder{
					Object: privatev1.ComputeInstanceTemplate_builder{
						Id:          templateId,
						Title:       "My template",
						Description: "My template.",
						AnsibleRole: "my_ansible_role",
					}.Build(),
				}.Build(),
			)
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(func() {
				_, err := templatesClient.Delete(
					ctx, privatev1.ComputeInstanceTemplatesDeleteRequest_builder{
						Id: templateId,
					}.Build(),
				)
				Expect(err).ToNot(HaveOccurred())
			})

			// Create a compute instance using that template:
			response, err := instancesClient.Create(ctx, publicv1.ComputeInstancesCreateRequest_builder{
				Object: publicv1.ComputeInstance_builder{
					Spec: publicv1.ComputeInstanceSpec_builder{
						Template:  templateId,
						Cores:     proto.Int32(2),
						MemoryGib: proto.Int32(4),
						BootDisk: publicv1.ComputeInstanceDisk_builder{
							SizeGib: 20,
						}.Build(),
						Image: publicv1.ComputeInstanceImage_builder{
							SourceRef: "quay.io/containerdisks/fedora:latest",
						}.Build(),
					}.Build(),
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			instance := response.GetObject()
			DeferCleanup(func() {
				_, err := instancesClient.Delete(ctx, publicv1.ComputeInstancesDeleteRequest_builder{
					Id: instance.GetId(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
			})

			// Check that the Kubernetes compute instance uses the Ansible role as the template identifier:
			kubeClient := tool.KubeClient()
			instanceList := &unstructured.UnstructuredList{}
			instanceList.SetGroupVersionKind(gvks.ComputeInstanceList)
			Eventually(
				func(g Gomega) {
					err := kubeClient.List(ctx, instanceList, crclient.MatchingLabels{
						labels.ComputeInstanceUuid: instance.GetId(),
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(instanceList.Items).To(HaveLen(1))
					resultId, found, err := unstructured.NestedString(
						instanceList.Items[0].Object, "spec", "templateID",
					)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(found).To(BeTrue())
					g.Expect(resultId).To(Equal("my_ansible_role"))
				},
				time.Minute,
				time.Second,
			).Should(Succeed())
		})

		It("Uses the template identifier directly when 'ansible_role' is not set", func() {
			// Create a template without an ansible_role:
			templateId := fmt.Sprintf("my_template_%s", uuid.New())
			_, err := templatesClient.Create(
				ctx, privatev1.ComputeInstanceTemplatesCreateRequest_builder{
					Object: privatev1.ComputeInstanceTemplate_builder{
						Id:          templateId,
						Title:       "My template",
						Description: "My template.",
					}.Build(),
				}.Build(),
			)
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(func() {
				_, err := templatesClient.Delete(
					ctx, privatev1.ComputeInstanceTemplatesDeleteRequest_builder{
						Id: templateId,
					}.Build(),
				)
				Expect(err).ToNot(HaveOccurred())
			})

			// Create a compute instance using that template:
			response, err := instancesClient.Create(ctx, publicv1.ComputeInstancesCreateRequest_builder{
				Object: publicv1.ComputeInstance_builder{
					Spec: publicv1.ComputeInstanceSpec_builder{
						Template:  templateId,
						Cores:     proto.Int32(2),
						MemoryGib: proto.Int32(4),
						BootDisk: publicv1.ComputeInstanceDisk_builder{
							SizeGib: 20,
						}.Build(),
						Image: publicv1.ComputeInstanceImage_builder{
							SourceRef: "quay.io/containerdisks/fedora:latest",
						}.Build(),
					}.Build(),
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			instance := response.GetObject()
			DeferCleanup(func() {
				_, err := instancesClient.Delete(ctx, publicv1.ComputeInstancesDeleteRequest_builder{
					Id: instance.GetId(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
			})

			// Check that the Kubernetes compute instance uses the template identifier:
			kubeClient := tool.KubeClient()
			instanceList := &unstructured.UnstructuredList{}
			instanceList.SetGroupVersionKind(gvks.ComputeInstanceList)
			Eventually(
				func(g Gomega) {
					err := kubeClient.List(ctx, instanceList, crclient.MatchingLabels{
						labels.ComputeInstanceUuid: instance.GetId(),
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(instanceList.Items).To(HaveLen(1))
					resultId, found, err := unstructured.NestedString(
						instanceList.Items[0].Object, "spec", "templateID",
					)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(found).To(BeTrue())
					g.Expect(resultId).To(Equal(templateId))
				},
				time.Minute,
				time.Second,
			).Should(Succeed())
		})
	})
})
