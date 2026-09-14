# Keep BusyBox for test consumers that use /bin/sh and the ls /tmp liveness probe.
# Alpine includes the CA certificate bundle needed by outbound TLS connections.
FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

# Argument for target architecture
ARG TARGETARCH

# Set the working directory
WORKDIR /

# Copy the application binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/magpiego /

# Set the entrypoint to the application binary
ENTRYPOINT ["/magpiego"]
