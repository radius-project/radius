FROM alpine
RUN apk --no-cache add ca-certificates
ADD dist/linux_amd64/release/radius-rp /
EXPOSE 5000
ENTRYPOINT ./radius-rp
