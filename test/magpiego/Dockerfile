# Keep BusyBox for test consumers that use /bin/sh and the ls /tmp liveness probe.
# Alpine includes the CA certificate bundle needed by outbound TLS connections.
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

# Argument for target architecture
ARG TARGETARCH

# Set the working directory
WORKDIR /

# Copy the application binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/magpiego /

# Set the entrypoint to the application binary
ENTRYPOINT ["/magpiego"]
