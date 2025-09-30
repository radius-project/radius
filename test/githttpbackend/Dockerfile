# syntax=docker/dockerfile:1

FROM alpine:3.20

ARG TARGETARCH

RUN apk add --no-cache git git-daemon ca-certificates

COPY ./linux_${TARGETARCH:-amd64}/release/githttpbackend /usr/local/bin/githttpbackend
RUN chmod +x /usr/local/bin/githttpbackend

ENV GIT_SERVER_TEMP_DIR=/var/lib/git
EXPOSE 3000

ENTRYPOINT ["/usr/local/bin/githttpbackend"]
