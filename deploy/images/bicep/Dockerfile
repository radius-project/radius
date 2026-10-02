FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

ARG TARGETARCH

# Copy the Bicep CLI binary and configuration files for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/bicep/bicepconfig.json bicepconfig.json
COPY ./linux_${TARGETARCH:-amd64}/release/bicep/bicep bicep

WORKDIR /

# Set the entrypoint to the Bicep CLI binary
CMD ["/bin/sh"]
