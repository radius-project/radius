FROM alpine:latest

# Argument for target architecture
ARG TARGETARCH

WORKDIR /

COPY ./linux_${TARGETARCH:-amd64}/release/bicep/bicepconfig.json bicepconfig.json

RUN apk --no-cache add curl
RUN curl -Lo bicep https://github.com/Azure/bicep/releases/latest/download/bicep-linux-x64 \
    && chmod +x ./bicep

ENTRYPOINT ["/bin/sh"]
