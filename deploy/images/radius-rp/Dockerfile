FROM alpine
RUN apk --no-cache add ca-certificates
COPY ./radius-rp /
EXPOSE 5000
ENTRYPOINT /radius-rp
