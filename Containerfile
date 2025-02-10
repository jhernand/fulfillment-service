FROM registry.access.redhat.com/ubi9/ubi:9.5-1739751568 AS builder

# Install packages:
RUN \
  dnf install -y \
  git \
  golang \
  python3.12 \
  python3.12-pip \
  unzip \
  && \
  dnf clean all -y

# Set the default Python version:
RUN \
  ln -s /usr/bin/python3.12 /usr/local/bin/python && \
  ln -s /usr/bin/pip3.12 /usr/local/bin/pip

# Set the working directory:
WORKDIR /source

# Install Python packages:
COPY requirements.txt /source
RUN pip install -r requirements.txt

# Copy the development tools and use them to prepare the environment:
COPY dev /source/dev
COPY dev.py /source
RUN ./dev.py setup

# Copy the rest of the source code and build the binary:
COPY . /source
RUN ./dev.py build

FROM registry.access.redhat.com/ubi9/ubi:9.5-1739751568 AS runtime

# Install the binary:
COPY --from=builder /source/bin/main /usr/local/bin
