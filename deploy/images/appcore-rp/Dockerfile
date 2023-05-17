FROM --platform=${TARGETPLATFORM:-linux/amd64} alpine:latest

ARG TARGETPLATFORM
ARG TARGETARCH

# Install ca-certificates
RUN apk --no-cache add ca-certificates

# Install Git (required for Terraform module downloads)
RUN apk --no-cache add git

# Create non-root user
RUN addgroup -g 65532 rpuser && \
    adduser -u 65532 -G rpuser -s /bin/sh -D rpuser

WORKDIR /

COPY ./linux_${TARGETARCH:-amd64}/release/appcore-rp /

USER rpuser

EXPOSE 8080
ENTRYPOINT ["/appcore-rp"]
