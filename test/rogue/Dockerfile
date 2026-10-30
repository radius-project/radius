FROM gcr.io/distroless/static-debian13:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99

# Argument for target architecture
ARG TARGETARCH

WORKDIR /

# Copy the rogue client binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/rogue-client /

# 65532:65532 is the default non-root user in distroless
USER 65532:65532

ENTRYPOINT ["/rogue-client"]
CMD ["idle"]
