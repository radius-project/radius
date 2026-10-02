# Keep BusyBox for test consumers that use /bin/sh and the ls /tmp liveness probe.
# Alpine includes the CA certificate bundle needed by outbound TLS connections.
FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

# Argument for target architecture
ARG TARGETARCH

# Set the working directory
WORKDIR /

# Copy the application binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/magpiego /

# Set the entrypoint to the application binary
ENTRYPOINT ["/magpiego"]
