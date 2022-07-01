FROM alpine
RUN apk --no-cache add ca-certificates

# Install libifxaudit
RUN wget https://packages.microsoft.com/centos/7/prod/libifxaudit-1.0-1525.x86_64.rpm && rpm -i libifxaudit-1.0-1525.x86_64.rpm

WORKDIR /app

RUN mkdir /app/config
COPY ./* /app/

RUN groupadd --gid 2000 radius

RUN useradd --home "/nonexistent" --shell "/sbin/nologin" --gid radius --uid 1000 radius

RUN chmod 770 /app/appcore-rp
RUN chown radius.radius -R /app

USER radius

EXPOSE 8080
ENTRYPOINT ["/app/appcore-rp"]