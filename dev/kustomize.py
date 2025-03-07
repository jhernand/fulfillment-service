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

import pathlib
import shutil
import tempfile

from . import command
from . import dirs
from . import tools

def build(
    image: str | None = None,
) -> pathlib.Path:
    """
    Run the 'kustomize' tool to generate the manifests. It doesn't modify the source file and return the name of a
    temporary file containing the results. The caller is responsible for removing this temporary file when it is no
    longer needed.
    """
    with tempfile.TemporaryDirectory() as tmp_dir:
        # Copy the source manifests to the temporary directory:
        shutil.copytree(
            src=dirs.project() / "manifests",
            dst=tmp_dir,
            dirs_exist_ok=True,
        )
        
        # If a image has been provided find the digest and run run the 'kustomize' tool to set it:
        if image is not None:
            code, image = command.eval(
                args=[
                    "podman",
                    "inspect",
                    f"--format={{{{ index .RepoDigests 0 }}}}",
                    image,
                ],
            )
            if code != 0:
                raise Exception(f"Failed to get image digest for image {image}")
            command.run(
                cwd=tmp_dir,
                args=[
                    tools.KUSTOMIZE.name,
                    "edit", "set", "image",
                    f"service={image}",
                ],
                check=True,
            )
        
        # Run the 'kustomize' tool to generate the manifests, and return the result:
        with tempfile.NamedTemporaryFile(delete=False) as manifests_file:
            command.run(
                cwd=tmp_dir,
                args=[
                   tools.KUSTOMIZE.name,
                   "build", ".",
                ],
                stdout=manifests_file,
            )
            return pathlib.Path(manifests_file.name)