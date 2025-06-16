# Use distroless image which already includes ca-certificates
FROM gcr.io/distroless/static:nonroot

# Argument for target architecture
ARG TARGETARCH

# Set the working directory
WORKDIR /

# Copy the rad CLI binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/rad /

# Set the user to non-root (65532:65532 is the default non-root user in distroless)
USER 65532:65532

# Set the entrypoint to the rad CLI binary
ENTRYPOINT ["/rad"]