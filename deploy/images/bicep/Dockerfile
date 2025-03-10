FROM alpine:latest AS base

ARG TARGETARCH
RUN if [ -z "${TARGETARCH}"]; then \
    TARGETARCH="amd64"; \
    fi

RUN apk --no-cache add curl

WORKDIR /

ARG BICEPARCH
RUN if [ "${TARGETARCH}" = "amd64" ]; then \
    BICEPARCH="x64"; \
    elif [ "${TARGETARCH}" = "arm64" ]; then \
    BICEPARCH="arm64"; \
    else \
    BICEPARCH=TARGETARCH; \
    fi

COPY ./linux_${TARGETARCH}/release/bicep/bicepconfig.json bicepconfig.json

RUN curl -Lo bicep https://github.com/Azure/bicep/releases/latest/download/bicep-linux-${BICEPARCH} \
    && chmod +x ./bicep

ENTRYPOINT ["/bin/sh"]
