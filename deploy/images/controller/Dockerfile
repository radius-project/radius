# Use a specific Debian slim image to enable running Bicep CLI
FROM debian:13.7-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2

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
