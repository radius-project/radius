# Use a specific Debian slim image to enable running Bicep CLI
FROM debian:bullseye-20250317-slim

ARG TARGETARCH

ENV DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1

# Install required dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        openssl \
    && update-ca-certificates \
    && rm -rf /var/lib/apt/lists/*

ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

# Create a non-root user
RUN useradd -m -s /bin/bash controlleruser && \
    mkdir -p /home/controlleruser && \
    chown -R controlleruser:controlleruser /home/controlleruser

# Copy the application binary
COPY ./linux_${TARGETARCH:-amd64}/release/controller /

# Set the user to non-root
USER controlleruser

# Set the working directory
WORKDIR /

# Copy the application binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/controller /

# Set the user to non-root (65532:65532 is the default non-root user in distroless)
USER 65532:65532

# Set the entrypoint to the application binary
ENTRYPOINT ["/controller"]
