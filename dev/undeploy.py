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

import click
import click_default_group

from . import command
from . import dirs
from . import kustomize
from . import tools

@click.group(
    cls=click_default_group.DefaultGroup,
    default="service",
    default_if_no_args=True,
)
def undeploy() -> None:
    """
    Undeploy components.
    """
    pass

@undeploy.command()
def service() -> None:
    """
    Undeploy from the K8S cluster specified in ~/.kube/config.
    """
    manifests_file = kustomize.build()
    try:
        command.run(
            args=[
                tools.KUBECTL.name,
                "delete",
                f"--ignore-not-found={True}",
                f"--filename={manifests_file}",
            ],
            check=True,
        )
    finally:
        manifests_file.unlink()

@undeploy.command()
def kind() -> None:
    """
    Delete the kind cluster.
    """
    config_file = dirs.project() / "kind.yaml"
    command.run(
        args=[
            tools.KIND.name,
            "delete", "cluster",
            "--name", "innabox",
        ],
        check=True,
    )