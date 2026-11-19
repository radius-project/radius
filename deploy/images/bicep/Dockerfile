FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

ARG TARGETARCH

# Copy the Bicep CLI binary and configuration files for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/bicep/bicepconfig.json bicepconfig.json
COPY ./linux_${TARGETARCH:-amd64}/release/bicep/bicep bicep

WORKDIR /

# Set the entrypoint to the Bicep CLI binary
CMD ["/bin/sh"]
