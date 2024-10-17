FROM ubuntu:latest

# Install tools needed  
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        curl \
        ca-certificates 
        
WORKDIR /

RUN curl -Lo bicep https://github.com/Azure/bicep/releases/latest/download/bicep-linux-arm64 \
    && chmod +x ./bicep

ENTRYPOINT ["/bin/sh"]