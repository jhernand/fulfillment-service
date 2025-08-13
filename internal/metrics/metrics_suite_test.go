/*
Copyright (c) 2025 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package metrics

import (
	"log/slog"
	"testing"

	"github.com/innabox/fulfillment-service/internal/logging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics")
}

// Logger used for tests:
var logger *slog.Logger

var _ = BeforeSuite(func() {
	var err error

	// Create a logger that writes to the Ginkgo writer, so that the log messages will be attached to the output of
	// the right test:
	logger, err = logging.NewLogger().
		SetLevel(slog.LevelDebug.String()).
		SetOut(GinkgoWriter).
		SetErr(GinkgoWriter).
		Build()
	Expect(err).ToNot(HaveOccurred())
})
