FROM gcr.io/distroless/static:nonroot
WORKDIR /

COPY ./radius-controller /

USER 65532:65532

ENTRYPOINT ["/radius-controller"]
