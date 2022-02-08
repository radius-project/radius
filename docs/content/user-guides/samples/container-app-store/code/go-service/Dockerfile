# syntax=docker/dockerfile:1

FROM golang:1.16-alpine as builder

WORKDIR /src
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY *.go ./
RUN go build -o /go-service

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
WORKDIR /app/
COPY --from=builder /go-service ./
EXPOSE 8050
CMD ["/app/go-service" ]  