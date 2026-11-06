FROM debian:bookworm-slim
ARG RAD_VERSION=0.61.1
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
 && curl -fsSL -o /usr/local/bin/rad https://github.com/radius-project/radius/releases/download/v${RAD_VERSION}/rad_linux_${TARGETARCH} \
 && chmod +x /usr/local/bin/rad && apt-get purge -y curl && apt-get autoremove -y && rm -rf /var/lib/apt/lists/*
ENV HOME=/home/argocd DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1
RUN mkdir -p /home/argocd/cmp-server/config && chown -R 999:999 /home/argocd
COPY --chown=999:999 plugin.yaml /home/argocd/cmp-server/config/plugin.yaml
COPY radius-generate.sh /usr/local/bin/radius-generate.sh
COPY --chown=999:999 prefetch /tmp/prefetch
USER 999
# Download Bicep and cache the Radius extension so the common case needs no network.
RUN cd /tmp/prefetch && rad bicep generate-kubernetes-manifest main.bicep -g x -d /tmp/o.yaml && rm -rf /tmp/prefetch /tmp/o.yaml
