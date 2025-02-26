FROM alpine:latest

ARG TARGETARCH

RUN apk --no-cache add curl

WORKDIR /

RUN curl -Lo bicep https://github.com/Azure/bicep/releases/latest/download/bicep-linux-x64 \
    && chmod +x ./bicep
    
ENTRYPOINT ["/bin/sh"]
