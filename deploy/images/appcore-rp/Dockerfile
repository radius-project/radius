FROM alpine
RUN apk --no-cache add ca-certificates

WORKDIR /app

RUN mkdir /app/
COPY ./* /app/

RUN groupadd --gid 2000 radius

RUN useradd --home "/nonexistent" --shell "/sbin/nologin" --gid radius --uid 1000 radius

RUN chmod 770 /app/appcore-rp
RUN chown radius.radius -R /app

USER radius

EXPOSE 8080
ENTRYPOINT ["/app/appcore-rp"]