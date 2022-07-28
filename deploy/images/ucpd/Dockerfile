# Note: distroless already includes ca-certificates
FROM gcr.io/distroless/static:nonroot
WORKDIR /

COPY ./ucpd /

USER 65532:65532

ENTRYPOINT ["/ucpd"]
