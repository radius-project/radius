FROM gcr.io/distroless/static:nonroot
WORKDIR /

COPY ./ucpd /

USER 65532:65532

ENTRYPOINT ["/ucpd"]
