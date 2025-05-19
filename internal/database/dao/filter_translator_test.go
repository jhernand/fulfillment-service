/*
Copyright (c) 2025 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package dao

import (
	"context"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/ginkgo/v2/dsl/table"
	. "github.com/onsi/gomega"

	testsv1 "github.com/innabox/fulfillment-service/internal/api/tests/v1"
)

var _ = Describe("Filter translator", func() {
	var (
		ctx        context.Context
		translator *FilterTranslator[*testsv1.Public]
	)

	BeforeEach(func() {
		var err error

		ctx = context.Background()

		translator, err = NewFilterTranslator[*testsv1.Public]().
			SetLogger(logger).
			Build()
		Expect(err).ToNot(HaveOccurred())
	})

	DescribeTable(
		"Translation",
		func(filter, expected string) {
			actual, err := translator.Translate(ctx, filter)
			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal(expected))
		},
		Entry(
			"Escape string with single quotes",
			`this.id == 'my \'value\''`,
			`id = e'my \'value\''`,
		),
		Entry(
			"Field equals value",
			"this.id == 'my value'",
			"id = 'my value'",
		),
		Entry(
			"Field not equals value",
			"this.id != 'my value'",
			"id != 'my value'",
		),
		Entry(
			"Integer greater than literal",
			"this.my_int32 > 42",
			"cast(public_data->>'my_int32' as integer) > 42",
		),
		Entry(
			"String in list",
			"this.my_string in ['a', 'b', 'c']",
			"public_data->>'my_string' in ('a', 'b', 'c')",
		),
		Entry(
			"Calculated value in list",
			"(this.my_int32 + 1) in [123, 456]",
			"cast(public_data->>'my_int32' as integer) + 1 in (123, 456)",
		),
		Entry(
			"String contains",
			`this.my_string.contains("my value")`,
			`public_data->>'my_string' like '%my value%'`,
		),
		Entry(
			"Nested string",
			`this.spec.spec_string == 'my_value'`,
			`public_data->'spec'->>'spec_string' = 'my_value'`,
		),
		Entry(
			"Creation timestamp is null",
			`this.metadata.creation_timestamp == null`,
			`creation_timestamp is null`,
		),
		Entry(
			"Creation timestamp is not null",
			`this.metadata.creation_timestamp != null`,
			`creation_timestamp is not null`,
		),
		Entry(
			"Deletion timestamp is null",
			`this.metadata.deletion_timestamp == null`,
			`nullif(deletion_timestamp, '1970-01-01 00:00:00Z') is null`,
		),
		Entry(
			"Deletion timestamp is not null",
			`this.metadata.deletion_timestamp != null`,
			`nullif(deletion_timestamp, '1970-01-01 00:00:00Z') is not null`,
		),
		Entry(
			"Timestamp in the past",
			`this.my_timestamp < now`,
			`cast(public_data->>'my_timestamp' as timestamp with time zone) < now()`,
		),
		Entry(
			"Timestamp in the future",
			`this.my_timestamp > now`,
			`cast(public_data->>'my_timestamp' as timestamp with time zone) > now()`,
		),
		Entry(
			"Reverse null and timestamp before null check",
			`null == this.my_timestamp`,
			`cast(public_data->>'my_timestamp' as timestamp with time zone) is null`,
		),
		Entry(
			"Reverse null and timestamp before not null check",
			`null != this.my_timestamp`,
			`cast(public_data->>'my_timestamp' as timestamp with time zone) is not null`,
		),
		Entry(
			"Check presence of identifier",
			`has(this.id)`,
			`true`,
		),
		Entry(
			"Check presence of creation timestamp",
			`has(this.metadata.creation_timestamp)`,
			`true`,
		),
		Entry(
			"Check presence of deletion timestamp",
			`has(this.metadata.deletion_timestamp)`,
			`deletion_timestamp != '1970-01-01 00:00:00Z'`,
		),
		Entry(
			"Check presence of boolean field",
			`has(this.my_bool)`,
			`public_data ? 'my_bool'`,
		),
		Entry(
			"Check presence of int32 field",
			`has(this.my_int32)`,
			`public_data ? 'my_int32'`,
		),
		Entry(
			"Check presence of int64 field",
			`has(this.my_int64)`,
			`public_data ? 'my_int64'`,
		),
		Entry(
			"Check presence of string field",
			`has(this.my_string)`,
			`public_data ? 'my_string'`,
		),
		Entry(
			"Check presence of float field",
			`has(this.my_float)`,
			`public_data ? 'my_float'`,
		),
		Entry(
			"Check presence of double field",
			`has(this.my_double)`,
			`public_data ? 'my_double'`,
		),
		Entry(
			"Check presence of timestamp field",
			`has(this.my_timestamp)`,
			`public_data ? 'my_timestamp'`,
		),
		Entry(
			"Check presence of message field",
			`has(this.spec)`,
			`public_data ? 'spec'`,
		),
		Entry(
			"Check presence of nested boolean field",
			`has(this.spec.spec_bool)`,
			`public_data->'spec' ? 'spec_bool'`,
		),
		Entry(
			"String starts with",
			`this.my_string.startsWith("my")`,
			`public_data->>'my_string' like 'my%'`,
		),
		Entry(
			"String ends with",
			`this.my_string.endsWith("my")`,
			`public_data->>'my_string' like '%my'`,
		),
		Entry(
			"Escape percent in like pattern",
			`this.my_string.startsWith("my%")`,
			`public_data->>'my_string' like 'my\%%'`,
		),
		Entry(
			"Escape underscore in like pattern",
			`this.my_string.startsWith("my_")`,
			`public_data->>'my_string' like 'my\_%'`,
		),
		Entry(
			"Check if object is deleted",
			`!has(this.metadata.deletion_timestamp)`,
			`not deletion_timestamp != '1970-01-01 00:00:00Z'`,
		),
	)
})
