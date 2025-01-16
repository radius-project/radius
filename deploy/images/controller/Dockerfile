# TODOWILLSMITH: Which base image should we use?
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

# # Set the user to non-root (65532:65532 is the default non-root user in distroless)
# USER 65532:65532

# Set the entrypoint to the application binary
ENTRYPOINT ["/controller"]
