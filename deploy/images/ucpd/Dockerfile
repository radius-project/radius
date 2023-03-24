# Note: distroless already includes ca-certificates
FROM --platform=$BUILDPLATFORM gcr.io/distroless/static:nonroot

ARG TARGETOS
ARG TARGETARCH

WORKDIR /

COPY ./linux_$TARGETARCH/release/ucpd /

USER 65532:65532
ENTRYPOINT ["/ucpd"]
