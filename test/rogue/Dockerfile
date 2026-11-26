FROM gcr.io/distroless/static-debian13:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5

# Argument for target architecture
ARG TARGETARCH

WORKDIR /

# Copy the rogue client binary for the specified architecture
COPY ./linux_${TARGETARCH:-amd64}/release/rogue-client /

# 65532:65532 is the default non-root user in distroless
USER 65532:65532

ENTRYPOINT ["/rogue-client"]
CMD ["idle"]
