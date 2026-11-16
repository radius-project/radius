FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

ARG TARGETARCH

# Copy the Bicep CLI binary and configuration files for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/bicep/bicepconfig.json bicepconfig.json
COPY ./linux_${TARGETARCH:-amd64}/release/bicep/bicep bicep

WORKDIR /

# Set the entrypoint to the Bicep CLI binary
CMD ["/bin/sh"]
