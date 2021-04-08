FROM mcr.microsoft.com/vscode/devcontainers/universal:1.3.0

WORKDIR /home/

COPY . .

# Install Azure CLI
USER root
RUN curl -sL https://aka.ms/InstallAzureCLIDeb | bash

# Install kubectl
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
RUN install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# Install rad CLI (Linux)
#    TODO: change to make binary directly inside this Dockerfile? 
RUN wget -O /usr/local/bin/rad --no-check-certificate https://radiuspublic.blob.core.windows.net/tools/rad/edge/linux-x64/rad
RUN chmod +rx /usr/local/bin/rad

# Download Radius VSCode extension
RUN wget -O /home/rad-vscode-bicep.vsix https://radiuspublic.blob.core.windows.net/tools/vscode/edge/rad-vscode-bicep.vsix

