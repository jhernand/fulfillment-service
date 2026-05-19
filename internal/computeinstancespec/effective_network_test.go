/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package computeinstancespec

import (
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/ginkgo/v2/dsl/table"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
)

var _ = Describe("Effective network attachments", func() {
	type Case struct {
		Spec    *privatev1.ComputeInstanceSpec
		Want    []*privatev1.NetworkAttachment
		WantErr bool
	}

	DescribeTable(
		"Resolves network attachments",
		func(c Case) {
			got, err := EffectiveNetworkAttachments(c.Spec)
			if c.WantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(cmp.Diff(c.Want, got, protocmp.Transform())).To(BeEmpty())
		},
		Entry(
			"Nil spec",
			Case{
				Spec:    nil,
				Want:    nil,
				WantErr: false,
			},
		),
		Entry(
			"Empty",
			Case{
				Spec:    privatev1.ComputeInstanceSpec_builder{}.Build(),
				Want:    nil,
				WantErr: false,
			},
		),
		Entry(
			"Legacy subnet empty string",
			Case{
				Spec: privatev1.ComputeInstanceSpec_builder{
					Subnet: proto.String(""),
				}.Build(),
				Want:    nil,
				WantErr: false,
			},
		),
		Entry(
			"Legacy subnet only",
			Case{
				Spec: privatev1.ComputeInstanceSpec_builder{
					Subnet: proto.String("sn-1"),
				}.Build(),
				Want: []*privatev1.NetworkAttachment{
					privatev1.NetworkAttachment_builder{Subnet: "sn-1"}.Build(),
				},
				WantErr: false,
			},
		),
		Entry(
			"Legacy subnet and security groups",
			Case{
				Spec: privatev1.ComputeInstanceSpec_builder{
					Subnet:         proto.String("sn-1"),
					SecurityGroups: []string{"sg-1", "sg-2"},
				}.Build(),
				Want: []*privatev1.NetworkAttachment{
					privatev1.NetworkAttachment_builder{
						Subnet:         "sn-1",
						SecurityGroups: []string{"sg-1", "sg-2"},
					}.Build(),
				},
				WantErr: false,
			},
		),
		Entry(
			"Legacy security groups only",
			Case{
				Spec: privatev1.ComputeInstanceSpec_builder{
					SecurityGroups: []string{"sg-1"},
				}.Build(),
				Want:    nil,
				WantErr: true,
			},
		),
		Entry(
			"Network attachments only",
			Case{
				Spec: privatev1.ComputeInstanceSpec_builder{
					NetworkAttachments: []*privatev1.NetworkAttachment{
						privatev1.NetworkAttachment_builder{Subnet: "a"}.Build(),
						privatev1.NetworkAttachment_builder{Subnet: "b"}.Build(),
					},
				}.Build(),
				Want: []*privatev1.NetworkAttachment{
					privatev1.NetworkAttachment_builder{Subnet: "a"}.Build(),
					privatev1.NetworkAttachment_builder{Subnet: "b"}.Build(),
				},
				WantErr: false,
			}),
		Entry(
			"Conflict legacy subnet with network attachments",
			Case{
				Spec: privatev1.ComputeInstanceSpec_builder{
					Subnet: proto.String("sn-1"),
					NetworkAttachments: []*privatev1.NetworkAttachment{
						privatev1.NetworkAttachment_builder{Subnet: "a"}.Build(),
					},
				}.Build(),
				Want:    nil,
				WantErr: true,
			},
		),
		Entry(
			"Conflict legacy security groups with network attachments",
			Case{
				Spec: privatev1.ComputeInstanceSpec_builder{
					SecurityGroups: []string{"sg-1"},
					NetworkAttachments: []*privatev1.NetworkAttachment{
						privatev1.NetworkAttachment_builder{Subnet: "a"}.Build(),
					},
				}.Build(),
				Want:    nil,
				WantErr: true,
			},
		),
		Entry(
			"Conflict deprecated subnet present but empty string with network_attachments",
			Case{
				Spec: privatev1.ComputeInstanceSpec_builder{
					Subnet: proto.String(""),
					NetworkAttachments: []*privatev1.NetworkAttachment{
						privatev1.NetworkAttachment_builder{Subnet: "a"}.Build(),
					},
				}.Build(),
				Want:    nil,
				WantErr: true,
			},
		),
	)
})
