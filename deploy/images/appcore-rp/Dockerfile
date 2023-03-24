# Note: distroless already includes ca-certificates
FROM --platform=$TARGETPLATFORM gcr.io/distroless/static:nonroot

ARG TARGETARCH

WORKDIR /

COPY ./linux_$TARGETARCH/release/appcore-rp /

USER 65532:65532

EXPOSE 8080
ENTRYPOINT ["/appcore-rp"]
