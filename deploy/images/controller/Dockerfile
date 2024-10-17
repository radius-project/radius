# Use distroless image which already includes ca-certificates
FROM ubuntu:latest

# Argument for target architecture
ARG TARGETARCH

# Set the working directory
WORKDIR /

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libicu-dev \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Copy the application binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/controller /
# TODO: delete once we know everything is working 
# COPY ./linux_arm64/release/controller /
COPY ./bicepconfig.json /bicepconfig.json
COPY ./test.bicep /test.bicep

# Set the entrypoint to the application binary
ENTRYPOINT ["/controller"]