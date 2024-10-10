# Use distroless image which already includes ca-certificates
FROM ubuntu

# Argument for target architecture
ARG TARGETARCH

# Set the working directory
WORKDIR /


# Install libicu
RUN apt-get update && apt-get install -y libicu-dev && rm -rf /var/lib/apt/lists/*

# Copy the application binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/controller /

COPY ./linux_${TARGETARCH:-amd64}/release/bicep-linux-x64 /bicep

# Set the user to non-root (65532:65532 is the default non-root user in distroless)
# USER 65532:65532

# Set the entrypoint to the application binary
ENTRYPOINT ["/controller"]
