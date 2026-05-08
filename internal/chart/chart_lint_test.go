/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package chart

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v2"
)

var _ = Describe("Service chart", func() {
	var (
		tempDir string
	)

	BeforeEach(func() {
		var err error

		// Create a temporary directory:
		tempDir, err = os.MkdirTemp("", "*.test")
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			err := os.RemoveAll(tempDir)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	It("Passes lint", func() {
		// Create a values file with all the required values:
		valuesFile := filepath.Join(tempDir, "values.yaml")
		valuesData := map[string]any{
			"certs": map[string]any{
				"caBundle": map[string]any{
					"configMap": "my-bundle",
				},
				"issuerRef": map[string]any{
					"kind": "ClusterIssuer",
					"name": "my-ca",
				},
			},
			"auth": map[string]any{
				"issuerUrl": "https://keycloak.example.com/realms/osac",
				"controllerCredentials": []any{
					map[string]any{
						"secret": map[string]any{
							"name": "my-secret",
							"items": []any{
								map[string]any{
									"key":   "id",
									"param": "client-id",
								},
								map[string]any{
									"key":   "secret",
									"param": "client-secret",
								},
							},
						},
					},
				},
			},
			"database": map[string]any{
				"connection": []any{
					map[string]any{
						"secret": map[string]any{
							"name": "db-creds",
							"items": []any{
								map[string]any{
									"key":   "uri",
									"param": "url",
								},
							},
						},
					},
				},
			},
			"idp": map[string]any{
				"url": "https://keycloak.example.com",
				"credentials": []any{
					map[string]any{
						"secret": map[string]any{
							"name": "my-secret",
							"items": []any{
								map[string]any{
									"key":   "id",
									"param": "client-id",
								},
								map[string]any{
									"key":   "secret",
									"param": "client-secret",
								},
							},
						},
					},
				},
			},
		}
		valuesBytes, err := yaml.Marshal(valuesData)
		Expect(err).ToNot(HaveOccurred())
		err = os.WriteFile(valuesFile, valuesBytes, 0600)
		Expect(err).ToNot(HaveOccurred())

		// Run the 'helm lint' command:
		listCmd := exec.Cmd{
			Dir:  projectDir,
			Path: helmPath,
			Args: []string{
				"helm",
				"lint",
				chartDir,
				"--values", valuesFile,
			},
			Stdout: GinkgoWriter,
			Stderr: GinkgoWriter,
		}
		err = listCmd.Run()
		Expect(err).ToNot(HaveOccurred())
	})
})
