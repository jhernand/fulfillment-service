/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package kafka

import (
	"github.com/spf13/pflag"
)

// AddFlags adds to the given flag set the flags needed to configure the Kafka tool:
//
//	--kafka-properties-file  Path to a properties file or directory containing Kafka client configuration.
//
// This flag can be specified multiple times. All files are loaded and merged together, with later values overriding
// earlier ones.
func AddFlags(flags *pflag.FlagSet) {
	flags.StringArray(
		propertiesFileFlagName,
		nil,
		"Path to a properties file or directory containing Kafka client configuration. "+
			"Can be specified multiple times; files are merged in order.",
	)
}

// Name of the flag:
const propertiesFileFlagName = "kafka-properties-file"
