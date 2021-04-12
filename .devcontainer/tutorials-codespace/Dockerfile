ARG VARIANT=3.1
FROM mcr.microsoft.com/vscode/devcontainers/dotnet:0-${VARIANT}

USER root

# Install library scripts & setup "first notice"
COPY library-scripts/* first-run-notice.txt /tmp/library-scripts/
RUN bash /tmp/library-scripts/azcli-debian.sh \
    && bash /tmp/library-scripts/kubectl-helm-debian.sh \
    && mv -f /tmp/library-scripts/first-run-notice.txt /usr/local/etc/vscode-dev-containers/ \
    && apt-get clean -y && rm -rf /var/lib/apt/lists/* /tmp/library-scripts        

# Install rad CLI (Linux)
#    TODO: change to make binary directly inside this Dockerfile? 
RUN wget -O /usr/local/bin/rad https://radiuspublic.blob.core.windows.net/tools/rad/edge/linux-x64/rad 
RUN chmod +rx /usr/local/bin/rad

# Download Radius VSCode extension
RUN wget -O /home/rad-vscode-bicep.vsix https://radiuspublic.blob.core.windows.net/tools/vscode/edge/rad-vscode-bicep.vsix

USER vscode

RUN touch ~/.vimrc