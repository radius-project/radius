ARG LDFLAGS="-w -s"

FROM golang:alpine AS build-env
RUN apk --no-cache add build-base git gcc
WORKDIR /src
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY . ./
RUN go build -o radius-rp -ldflags "$LD_FLAGS" cmd/rp/main.go

FROM alpine
RUN apk --no-cache add ca-certificates
COPY --from=build-env /src/radius-rp ./radius-rp
EXPOSE 5000
ENTRYPOINT ./radius-rp