# Keep BusyBox for test consumers that use /bin/sh and the ls /tmp liveness probe.
# Alpine includes the CA certificate bundle needed by outbound TLS connections.
FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

# Argument for target architecture
ARG TARGETARCH

# Set the working directory
WORKDIR /

# Copy the application binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/magpiego /

# Set the entrypoint to the application binary
ENTRYPOINT ["/magpiego"]
