# Note: distroless already includes ca-certificates
FROM --platform=${TARGETPLATFORM:-linux/amd64} gcr.io/distroless/static:nonroot

ARG TARGETPLATFORM
ARG TARGETARCH

WORKDIR /

COPY ./linux_${TARGETARCH:-amd64}/release/ucpd /

USER 65532:65532
ENTRYPOINT ["/ucpd"]
