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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
)

var _ = Describe("applyFieldDefinitions", func() {
	It("rejects editable field with no default and no user value", func() {
		spec := &privatev1.ClusterSpec{}
		fieldDefs := []*privatev1.FieldDefinition{{
			Path:     "pull_secret",
			Editable: true,
		}}
		err := applyFieldDefinitions(spec, fieldDefs)
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		Expect(err.Error()).To(ContainSubstring("pull_secret"))
	})

	It("accepts editable field with no default when user provides value", func() {
		pullSecret := "my-secret"
		spec := &privatev1.ClusterSpec{
			PullSecret: &pullSecret,
		}
		fieldDefs := []*privatev1.FieldDefinition{{
			Path:     "pull_secret",
			Editable: true,
		}}
		err := applyFieldDefinitions(spec, fieldDefs)
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.GetPullSecret()).To(Equal("my-secret"))
	})

	It("applies default for editable field when user provides no value", func() {
		spec := &privatev1.ClusterSpec{}
		defaultVal, err := structpb.NewValue("default-secret")
		Expect(err).ToNot(HaveOccurred())
		fieldDefs := []*privatev1.FieldDefinition{{
			Path:     "pull_secret",
			Editable: true,
			Default:  defaultVal,
		}}
		err = applyFieldDefinitions(spec, fieldDefs)
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.GetPullSecret()).To(Equal("default-secret"))
	})

	It("overrides user value with default for non-editable field", func() {
		userValue := "user-value"
		spec := &privatev1.ClusterSpec{
			PullSecret: &userValue,
		}
		defaultVal, err := structpb.NewValue("admin-value")
		Expect(err).ToNot(HaveOccurred())
		fieldDefs := []*privatev1.FieldDefinition{{
			Path:     "pull_secret",
			Editable: false,
			Default:  defaultVal,
		}}
		err = applyFieldDefinitions(spec, fieldDefs)
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.GetPullSecret()).To(Equal("admin-value"))
	})

	It("returns no error for empty field definitions", func() {
		pullSecret := "my-secret"
		spec := &privatev1.ClusterSpec{
			PullSecret: &pullSecret,
		}
		err := applyFieldDefinitions(spec, nil)
		Expect(err).ToNot(HaveOccurred())
	})

	It("rejects when any required field is missing among multiple fields", func() {
		sshKey := "my-ssh-key"
		spec := &privatev1.ClusterSpec{
			SshPublicKey: &sshKey,
		}
		defaultRelease, err := structpb.NewValue("4.16")
		Expect(err).ToNot(HaveOccurred())
		fieldDefs := []*privatev1.FieldDefinition{
			{
				Path:     "release_image",
				Editable: true,
				Default:  defaultRelease,
			},
			{
				Path:     "pull_secret",
				Editable: true,
			},
			{
				Path:     "ssh_public_key",
				Editable: true,
			},
		}
		err = applyFieldDefinitions(spec, fieldDefs)
		Expect(err).To(HaveOccurred())
		Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		Expect(err.Error()).To(ContainSubstring("pull_secret"))
	})

	It("applies is_windows field definition default to compute instance spec", func() {
		spec := &privatev1.ComputeInstanceSpec{}
		defaultVal, err := structpb.NewValue(true)
		Expect(err).ToNot(HaveOccurred())
		fieldDefs := []*privatev1.FieldDefinition{{
			Path:     "is_windows",
			Editable: true,
			Default:  defaultVal,
		}}
		err = applyFieldDefinitions(spec, fieldDefs)
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.GetIsWindows()).To(BeTrue())
	})

	It("forces is_windows value for non-editable field definition", func() {
		falseVal := false
		spec := &privatev1.ComputeInstanceSpec{
			IsWindows: &falseVal,
		}
		defaultVal, err := structpb.NewValue(true)
		Expect(err).ToNot(HaveOccurred())
		fieldDefs := []*privatev1.FieldDefinition{{
			Path:     "is_windows",
			Editable: false,
			Default:  defaultVal,
		}}
		err = applyFieldDefinitions(spec, fieldDefs)
		Expect(err).ToNot(HaveOccurred())
		Expect(spec.GetIsWindows()).To(BeTrue())
	})
})

var _ = Describe("addPublishedFilter", func() {
	var server *ClusterCatalogItemsServer

	BeforeEach(func() {
		server = &ClusterCatalogItemsServer{}
	})

	DescribeTable("composes filter correctly",
		func(input string, expected string) {
			result, err := server.addPublishedFilter(input)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(expected))
		},
		Entry("empty filter", "", "this.published"),
		Entry("simple filter", "this.id == '123'", "(this.id == '123') && this.published"),
		Entry("compound filter", "this.title == 'a' && this.template == 'b'",
			"(this.title == 'a' && this.template == 'b') && this.published"),
		Entry("valid filter with OR is safely composed", "true || true",
			"(true || true) && this.published"),
	)

	DescribeTable("rejects malformed filters",
		func(input string) {
			_, err := server.addPublishedFilter(input)
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		},
		Entry("unbalanced parens to bypass published", `true) || (true`),
		Entry("unbalanced closing paren", `true)`),
		Entry("unbalanced opening paren", `(true`),
	)

	DescribeTable("validateCELSyntax",
		func(input string, shouldPass bool) {
			err := validateCELSyntax(input)
			if shouldPass {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
			}
		},
		Entry("valid simple expression", "true", true),
		Entry("valid field reference", "this.published", true),
		Entry("valid comparison", "this.id == '123'", true),
		Entry("valid compound", "this.a && this.b || this.c", true),
		Entry("unbalanced closing paren", "true)", false),
		Entry("unbalanced opening paren", "(true", false),
		Entry("injection attempt", `true) || (true`, false),
		Entry("empty string is not valid CEL", "", false),
	)
})
