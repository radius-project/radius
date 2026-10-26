# Use distroless image which already includes ca-certificates
FROM gcr.io/distroless/static-debian13:nonroot@sha256:d62067879de890a1def9da0e7766b7d20810a5e5555cef8506b752437d7a3e88

# Argument for target architecture
ARG TARGETARCH

# Set the working directory
WORKDIR /

# Copy the application binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/pre-upgrade /

# Set the user to non-root (65532:65532 is the default non-root user in distroless)
USER 65532:65532

# Set the entrypoint to the application binary
ENTRYPOINT ["/pre-upgrade"]
