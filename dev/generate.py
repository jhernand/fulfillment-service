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

import logging
import os
import pathlib
import shutil
import tempfile

import click

from . import buf
from . import command
from . import defaults
from . import dirs
from . import tools

@click.group(invoke_without_command=True)
@click.pass_context
def generate(ctx: click.Context):
    """
    Generate code.
    """
    if ctx.invoked_subcommand is not None:
        return
    ctx.invoke(grpc)

@generate.command()
def grpc() -> None:
    """
    Generate gRPC artifacts.
    """
    logging.info("Generating gRPC artifacts")
    
    tmp_dir = pathlib.Path(tempfile.mkdtemp())
    try:
        # Download API definition:
        logging.info(f"Downloading API definition")
        tar_name = f"v{defaults.API_VERSION}.tar.gz"
        tar_url = (
            "https://github.com"
            f"/innabox/fulfillment-api/archive/refs/tags"
            f"/{tar_name}"
        )
        tar_file = tmp_dir / tar_name
        command.run(
            args=[
                "curl",
                "--location",
                "--silent",
                "--fail",
                "--output", str(tar_file),
                tar_url,
            ],
            check=True,
        )
        command.run(args=[
            "tar",
            "--directory", str(tmp_dir),
            "--extract",
            "--file", str(tar_file),
            "--strip-components", "1",
            f"fulfillment-api-{defaults.API_VERSION}",
        ])

        # Create or clean the output directory:
        out_dir = dirs.project() / "internal" / "api"
        if out_dir.exists():
            shutil.rmtree(out_dir)
        os.makedirs(out_dir)
        
        # Run the 'buf' tool to generate the code:
        logging.info(f"Generating API code")
        command.run(
            cwd=tmp_dir,
            args=[
                tools.BUF.name,
                "generate",
                "--template", buf.gen_yaml(out_dir=out_dir),
            ],
            check=True,
        )
    finally:
        shutil.rmtree(tmp_dir)

@generate.command()
def mocks() -> None:
    """
    Generate mocks.
    """
    command.run(args=["go", "generate", "./..."])