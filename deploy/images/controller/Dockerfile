# Use a specific Debian slim image to enable running Bicep CLI
FROM debian:bullseye-20250317-slim

ARG TARGETARCH

# Create non-root user in a single RUN command to minimize layers
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libicu67 \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/bash controlleruser && \
    mkdir -p /home/controlleruser && \
    chown -R controlleruser:controlleruser /home/controlleruser

# Copy the application binary
COPY ./linux_${TARGETARCH:-amd64}/release/controller /

# # Set the user to non-root
USER controlleruser

# Set the working directory
WORKDIR /

# Set the entrypoint to the application binary
ENTRYPOINT ["/controller"]
