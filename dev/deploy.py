# -*- coding: utf-8 -*-

#
# Copyright (c) 2025 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except
# in compliance with the License. You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software distributed under the License
# is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express
# or implied. See the License for the specific language governing permissions and limitations under
# the License.
#

import click
import click_default_group

from . import command
from . import defaults
from . import dirs
from . import kustomize
from . import tools

@click.group(
    cls=click_default_group.DefaultGroup,
    default="service",
    default_if_no_args=True,
)
def deploy() -> None:
    """
    Deploy components.
    """
    pass

@deploy.command()
@click.option(
    "--image",
    help="Image reference.",
    default=f"{defaults.IMAGE_REPOSITORY}:{defaults.IMAGE_TAG}",
)
def service(
    image: str,
) -> None:
    """
    Deploy to the K8S cluster specified in ~/.kube/config.
    """
    manifests_file = kustomize.build(image=image)
    try:
        command.run(
            args=[
                tools.KUBECTL.name,
                "apply",
                "--filename", str(manifests_file),
            ],
            check=True,
        )
    finally:
        manifests_file.unlink()

@deploy.command()
def kind() -> None:
    """
    Create a kind cluster.
    """
    # Create the cluster:
    config_file = dirs.project() / "kind.yaml"
    command.run(
        args=[
            tools.KIND.name,
            "create", "cluster",
            "--config", str(config_file),
        ],
        check=True,
    )
    
    # Install the ingress controller:
    command.run(
        args=[
            tools.KUBECTL.name,
            "apply",
            "--filename", "https://projectcontour.io/quickstart/contour.yaml",
        ],
        check=True,
    )

    # Install the certificate manager:
    command.run(
        args=[
            tools.KUBECTL.name,
            "apply",
            "--filename",
            "https://github.com/cert-manager/cert-manager/releases/download/v1.12.16/cert-manager.yaml",
        ],
        check=True,
    )