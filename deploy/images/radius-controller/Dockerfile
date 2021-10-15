FROM gcr.io/distroless/static:nonroot
WORKDIR /

COPY dist/linux_amd64/release/radius-controller /

USER 65532:65532

ENTRYPOINT ["/radius-controller"]
