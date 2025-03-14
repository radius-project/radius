# Use Ubuntu image to enable Bicep CLI.
# Switch to something more lightweight when we can find a 
# base image that supports running Bicep. 
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

# Set the entrypoint to the application binary
ENTRYPOINT ["/controller"]
