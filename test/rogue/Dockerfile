FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

# Argument for target architecture
ARG TARGETARCH

WORKDIR /

# Copy the rogue client binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/rogue-client /

# 65532:65532 is the default non-root user in distroless
USER 65532:65532

ENTRYPOINT ["/rogue-client"]
CMD ["idle"]
