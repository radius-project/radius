# Note: distroless already includes ca-certificates
FROM gcr.io/distroless/static:nonroot
WORKDIR /

COPY ./* /

USER 65532:65532

EXPOSE 8080
ENTRYPOINT ["/appcore-rp"]
