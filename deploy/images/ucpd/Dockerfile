# Use distroless image which already includes ca-certificates
FROM gcr.io/distroless/static-debian13:nonroot@sha256:516757106de35205efb70b91ba03f6f1298424a0678c603255cf2852a7000087

# Argument for target architecture
ARG TARGETARCH

# Set the working directory
WORKDIR /

# Copy the application binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/ucpd /

# Copy the manifest files for the built-in providers
COPY ./manifest/built-in-providers/ /manifest/built-in-providers/

# Set the user to non-root (65532:65532 is the default non-root user in distroless)
USER 65532:65532

# Set the entrypoint to the application binary
ENTRYPOINT ["/ucpd"]
