# Use Alpine image to enable Git and Docker CLI installation.
# Git is required for Terraform module downloads.
# Docker CLI is required for Terraform docker provider (container image builds).
FROM alpine:3.20

ARG TARGETARCH

# Install ca-certificates, Git, and Docker CLI
# Create non-root user in a single RUN command to minimize layers
RUN apk --no-cache add ca-certificates git docker-cli && \
    addgroup -g 65532 rpuser && \
    adduser -u 65532 -G rpuser -s /bin/sh -D rpuser && \
    addgroup rpuser docker 2>/dev/null || true

WORKDIR /

# Copy the application binary
COPY ./linux_${TARGETARCH:-amd64}/release/dynamic-rp /

# Set the user to non-root
USER rpuser

# Expose the application port
EXPOSE 8080

# Set the entrypoint to the application binary
ENTRYPOINT ["/dynamic-rp"]
