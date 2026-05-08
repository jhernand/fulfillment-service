/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package chart

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"

	"github.com/osac-project/fulfillment-service/internal/logging"
)

func TestChart(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Helm chart")
}

// Logger used for tests:
var logger *slog.Logger

// Location of the 'helm' command:
var helmPath string

// Chart files:
var (
	projectDir string
	chartDir   string
	chartFile  string
)

var _ = BeforeSuite(func() {
	var err error

	// Create the logger:
	logger, err = logging.NewLogger().
		SetLevel(slog.LevelDebug.String()).
		SetWriter(GinkgoWriter).
		Build()
	Expect(err).ToNot(HaveOccurred())

	// Check that the 'helm' command is available:
	helmPath, err = exec.LookPath("helm")
	Expect(err).ToNot(
		HaveOccurred(),
		"The 'helm' command isn't available",
	)

	// Check that the 'helm' command is at least version 4:
	helmOut := &bytes.Buffer{}
	helmCmd := exec.Cmd{
		Path: helmPath,
		Args: []string{
			"helm",
			"version",
			"--template", "{{ .Version }}",
		},
		Stdout: io.MultiWriter(helmOut, GinkgoWriter),
		Stderr: GinkgoWriter,
	}
	err = helmCmd.Run()
	Expect(err).ToNot(HaveOccurred())
	helmVersion := strings.TrimSpace(helmOut.String())
	Expect(helmVersion).To(
		MatchRegexp("^v4\\."),
		"Helm version is '%s' but must be at least version 4",
		helmVersion,
	)

	// Find the chart directory:
	currentDir, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())
	projectDir = filepath.Join(currentDir, "..", "..")
	chartDir = filepath.Join(projectDir, "charts", "service")
	chartDir, err = filepath.Abs(chartDir)
	Expect(err).ToNot(HaveOccurred())

	// Check that it is indeed the chart directory:
	chartFile = filepath.Join(chartDir, "Chart.yaml")
	_, err = os.Stat(chartFile)
	Expect(err).ToNot(
		HaveOccurred(),
		"Chart file '%s' doesn't exist",
		chartFile,
	)
})
