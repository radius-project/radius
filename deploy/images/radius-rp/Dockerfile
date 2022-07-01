FROM alpine
RUN apk --no-cache add ca-certificates

# Install libifxaudit
RUN wget https://packages.microsoft.com/centos/7/prod/libifxaudit-1.0-1525.x86_64.rpm && rpm -i libifxaudit-1.0-1525.x86_64.rpm

COPY ./radius-rp /
EXPOSE 5000
ENTRYPOINT /radius-rp
