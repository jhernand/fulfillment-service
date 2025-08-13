/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

// This file contains functions that calculate the labels included in metrics.

package metrics

import (
	"strconv"
)

// codeLabel calculates the `code` label from the given gRPC response code.
func codeLabel(code int) string {
	return strconv.Itoa(code)
}

// Names of the labels added to metrics:
const (
	codeLabelName   = "code"
	methodLabelName = "method"
)

// Array of labels added to call metrics:
var callLabelNames = []string{
	methodLabelName,
	codeLabelName,
}
