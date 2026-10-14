FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

# Argument for target architecture
ARG TARGETARCH

WORKDIR /

# Copy the rogue client binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/rogue-client /

# 65532:65532 is the default non-root user in distroless
USER 65532:65532

ENTRYPOINT ["/rogue-client"]
CMD ["idle"]
