# -*- coding: utf-8 -*-

#
# Copyright (c) 2025 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
# the License. You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
# specific language governing permissions and limitations under the License.
#

class Tool:
    def __init__(
        self,
        name: str,
        version: str = "",
        version_command: list[str] = [],
        version_pattern: str = "",
        checksums: dict[str, str] = {},
    ):
        # Save the values:
        self.name = name
        self.version = version
        self.version_command = version_command
        self.version_pattern = version_pattern
        
        # The keys of the checksums dictionary can reference the name or the version:
        self.checksums = {}
        for artifact_name, artifact_checksum in checksums.items():
            artifact_name = artifact_name.format(
                name=name,
                version=version,
            )
            self.checksums[artifact_name] = artifact_checksum

BUF = Tool(
    name="buf",
    version="1.50.0",
    version_command=["buf", "--version"],
    version_pattern=r"^(?P<version>.*)$",
    checksums={
        "sha256.txt": "736e74d1697dcf253bc60b2f0fb4389c39dbc7be68472a7d564a953df8b19d12",
    },
)

GO = Tool(
    name="go",
    version="1.22.9",
    version_command=["go", "env", "GOVERSION"],
    version_pattern=r"^go(?P<version>\d+(\.\d+)+).*$",
    checksums={
        "{name}-{version}-checksums.txt": "f2313a5ce4330c00989cd22da3494b3717d7bfa734349b111cabe281bf100aef",
    },
)

GOLANGCI_LINT = Tool(
    name="golangci-lint",
    version="1.63.4",
    version_command=["golangci-lint", "version"],
    version_pattern=r"^.*version\s+(?P<version>.+)\s+built.*$",
    checksums={
        "{name}-{version}-checksums.txt": "f2313a5ce4330c00989cd22da3494b3717d7bfa734349b111cabe281bf100aef",
    },
)

GINKGO = Tool(
    name="ginkgo",
    version="2.22.2",
    version_command=["ginkgo", "version"],
    version_pattern=r"Ginkgo Version\s+(?P<version>.*)$",
)

GPRCURL = Tool(
    name="grpcurl",
    version="1.9.2",
    version_command=["golangci-lint", "version"],
    version_pattern=r"^grpcurl\s+v(?P<version>.+)$",
    checksums={
        "{name}_{version}_checksums.txt": "db4ad37669b7d61d018484981887db0e72ba9809c9651d72aef7c0d9deb848fc",
    },
)

KUBECTL = Tool(
    name="kubectl",
    version="1.32.2",
    version_command=["kubectl", "version", "--client"],
    version_pattern=r"^Client Version:\s+v(?P<version>.+)$",
    checksums={
        "linux/amd64/kubectl": "4f6a959dcc5b702135f8354cc7109b542a2933c46b808b248a214c1f69f817ea",
    },
)

KUSTOMIZE = Tool(
    name="kustomize",
    version="5.6.0",
    version_command=["kustomize", "version"],
    version_pattern=r"^v(?P<version>.+)$",
    checksums={
        "checksums.txt": "fe3af8dd44755966538bcf20f9a52875ae3a2a80452e9ef9c187c96ea3ad4ede",
    },
)

PROTOC = Tool(
    name="protoc",
    version="29.3",
    version_command=["protoc", "--version"],
    version_pattern=r"^libprotoc\s+(?P<version>.*)$",
    checksums={
        "{name}-{version}-linux-x86_64.zip": "3e866620c5be27664f3d2fa2d656b5f3e09b5152b42f1bedbf427b333e90021a",
    },
)

PROTOC_GEN_GO = Tool(
    name="protoc-gen-go",
    version="1.36.4",
    version_command=["protoc-gen-go", "--version"],
    version_pattern=r"^protoc-gen-go\s+v(?P<version>.*)$",
)

PROTOC_GEN_GO_GRPC = Tool(
    name="protoc-gen-go-grpc",
    version_command=["protoc-gen-go-grpc", "--version"],
    version_pattern=r"^protoc-gen-go-grpc\s+(?P<version>.*)$",
    version="1.5.1",
)