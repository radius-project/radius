# Use Alpine image to enable Git installation for Terraform module downloads.
# Switch to distroless when Terraform execution is moved to a separate container.
FROM alpine:3.21.3

ARG TARGETARCH

# Install ca-certificates and Git (required for Terraform module downloads)
# Create non-root user in a single RUN command to minimize layers
RUN apk --no-cache add ca-certificates git && \
    addgroup -g 65532 rpuser && \
    adduser -u 65532 -G rpuser -s /bin/sh -D rpuser

WORKDIR /

# Copy the application binary
COPY ./linux_${TARGETARCH:-amd64}/release/applications-rp /

# Set the user to non-root
USER rpuser

# Expose the application port
EXPOSE 8080

# Set the entrypoint to the application binary
ENTRYPOINT ["/applications-rp"]
