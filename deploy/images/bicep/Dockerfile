FROM alpine:3.21.3

ARG TARGETARCH

WORKDIR /

COPY ./linux_${TARGETARCH:-amd64}/release/bicep/bicepconfig.json bicepconfig.json

# Install curl (required for downloading Bicep CLI) and create a non-root user
# Create non-root user in a single RUN command to minimize layers
RUN apk --no-cache add curl && \
    curl -Lo bicep https://github.com/Azure/bicep/releases/latest/download/bicep-linux-musl-x64 && \
    chmod +x ./bicep

ENTRYPOINT ["/bin/sh"]
