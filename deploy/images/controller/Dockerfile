# Use a specific Debian slim image to enable running Bicep CLI
FROM debian:13.6-slim@sha256:d7e12182ce18b85b93007c1dedf31f2d29e01ccf3182cc4017c709b6259bc132

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

# Set the entrypoint to the application binary
ENTRYPOINT ["/controller"]
