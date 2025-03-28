FROM alpine:3.21.3

ARG TARGETARCH

# Copy the Bicep CLI binary and configuration files for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/bicep/bicepconfig.json bicepconfig.json
COPY ./linux_${TARGETARCH:-amd64}/release/bicep/bicep bicep

WORKDIR /

# Set the entrypoint to the Bicep CLI binary
CMD ["/bin/sh"]
