# This is the base image, both for the builder and runtime containers.
ARG BASE=registry.access.redhat.com/ubi10/ubi:10.1-1773895909

# First stage is to build the image that contains the image with all the development tools needed to build the binary.
FROM ${BASE} AS builder

# Set this to 'true' to build the binary with debugging symbols and disabling optimizations.
ARG DEBUG=false

# TODO: Currently we install Go from a tarball because the package for UBI 10 seems to be broken:
#
# podman build --no-cache .
# [1/2] STEP 1/9: FROM registry.access.redhat.com/ubi10/ubi:10.1-1773895909 AS builder
#
# ...
# Error:
#  Problem: package golang-bin-1.26.2-2.el10_2.x86_64 from ubi-10-for-x86_64-appstream-rpms requires gcc, but none of the providers can be installed
#   - package golang-1.26.2-2.el10_2.x86_64 from ubi-10-for-x86_64-appstream-rpms requires golang-bin = 1.26.2-2.el10_2, but none of the providers can be installed
#   - package gcc-14.3.1-4.4.el10.x86_64 from ubi-10-for-x86_64-appstream-rpms requires glibc-devel >= 2.2.90-12, but none of the providers can be installed
#   - conflicting requests
#   - nothing provides glibc = 2.39-124.el10_2 needed by glibc-devel-2.39-124.el10_2.x86_64 from ubi-10-for-x86_64-appstream-rpms
# (try to add '--skip-broken' to skip uninstallable packages or '--nobest' to use not only best candidate packages)
# Error: building at STEP "RUN set -e;   pkgs=(git golang);   dnf install -y "${pkgs[@]}";   dnf clean all -y": while running runtime: exit status 1
#
# When there is a solution for that we s hould restore the 'golang' package below, and remove this.
RUN \
  curl -Lo /tmp/tarball https://dl.google.com/go/go1.25.10.linux-amd64.tar.gz && \
  echo 42d4f7a32316aa66591eca7e89867256057a4264451aca10570a715b3637ba70 /tmp/tarball | sha256sum -c && \
  tar -C /usr/local -x -f /tmp/tarball && \
  rm /tmp/tarball
ENV \
  PATH="${PATH}:/usr/local/go/bin"

# Install packages:
RUN \
  set -e; \
  pkgs=(git); \
  dnf install -y "${pkgs[@]}"; \
  dnf clean all -y

# Copy only the 'go.mod' and 'go.sum' files and try to download the required modules, so that hopefully this will be
# in a layer that can be cached reused for builds that don't change the dependencies.
WORKDIR /source
COPY go.mod go.sum /source/
RUN \
  set -e; \
  go mod download

# Version can be passed as a build arg to avoid requiring git inside the container.
# This is necessary when building from a git worktree, where the .git reference
# cannot be resolved inside the container context.
ARG VERSION=""

# Copy the rest of the source and build the binary:
COPY . /source/
RUN \
  set -e; \
  version="${VERSION}"; \
  if [[ -z "${version}" ]]; then \
    version=$(git describe --tags --always 2>/dev/null || echo "dev"); \
  fi; \
  gcflags=""; \
  ldflags="-X github.com/osac-project/fulfillment-service/internal/version.id=${version}"; \
  if [[ "${DEBUG}" == "true" ]]; then \
    gcflags="all=-N -l"; \
  fi; \
  go build -gcflags="${gcflags}" -ldflags="${ldflags}" ./cmd/fulfillment-service

# Second stage is to build the image that contains the binary, without the development tools, except the debugger
# if enabled.
FROM ${BASE} AS runtime

# Set this to 'true' to include the 'dlv' debugger in the image.
ARG DEBUG=false

# Install packages:
RUN \
  set -e; \
  pkgs=(openssl); \
  if [[ "${DEBUG}" == "true" ]]; then \
    pkgs+=(delve); \
  fi; \
  dnf install -y "${pkgs[@]}"; \
  dnf clean all -y

# Install the binary:
COPY --from=builder /source/fulfillment-service /usr/local/bin
