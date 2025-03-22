# Use Alpine image to enable running Bicep CLI
FROM alpine:3.21.3

ARG TARGETARCH

# Install ca-certificates and icu-libs (required for running Bicep CLI)
# Create non-root user in a single RUN command to minimize layers
RUN apk --no-cache add ca-certificates icu-libs && \
    addgroup -g 65532 controlleruser && \
    adduser -u 65532 -G controlleruser -s /bin/sh -D controlleruser

WORKDIR /

# Copy the application binary
COPY ./linux_${TARGETARCH:-amd64}/release/controller /

# Set the user to non-root
USER controlleruser

# Set the entrypoint to the application binary
ENTRYPOINT ["/controller"]
