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

import functools
import os
import pathlib

@functools.cache
def project() -> pathlib.Path:
    """
    Returns the root directory of the project.
    """
    return pathlib.Path(__file__).parent.parent

def bin() -> pathlib.Path:
    """
    Returns the bin directory of the project, where the generated binaries will be placed.
    """
    return project() / "bin"

def dev() -> pathlib.Path:
    """
    Returns the 'dev' directory of the project, where the development tools (build scripts, etc) are placed.
    """
    return project() / "dev"

@functools.cache
def local() -> pathlib.Path:
    """
    Retruns the local directory for installation of tools.
    """
    # Use the system local directory if it exists and it is writeable:
    local_dir = pathlib.Path("/usr/local")
    if local_dir.exists() and os.access(local_dir, os.W_OK):
        return local_dir

    # Use the project specific local directory it exists or if it is possible to create it:
    project_dir = project()
    local_dir = project_dir.parent / ".local"
    if local_dir.exists() or os.access(project_dir.parent, os.W_OK):
        return local_dir

    # If we are here then we failed to find a suitable local directory:
    raise Exception("Failed to select a suitable local directory")

@functools.cache
def local_bin() -> pathlib.Path:
    """
    Retruns the local directory for installation of tool binaries.
    """
    return local() / "bin"
