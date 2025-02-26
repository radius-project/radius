FROM ubuntu:latest

# Argument for target architecture
ARG TARGETARCH

# Set the working directory
WORKDIR /

# Install the required dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libicu-dev \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Copy the application binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/controller /

# Copy the bicepconfig.json file
COPY bicepconfig-controller.json /bicepconfig.json

# Set the entrypoint to the application binary
ENTRYPOINT ["/controller"]
