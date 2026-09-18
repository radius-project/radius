FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

ARG TARGETARCH

# Copy the Bicep CLI binary and configuration files for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/bicep/bicepconfig.json bicepconfig.json
COPY ./linux_${TARGETARCH:-amd64}/release/bicep/bicep bicep

WORKDIR /

# Set the entrypoint to the Bicep CLI binary
CMD ["/bin/sh"]
