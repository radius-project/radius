FROM golang:alpine AS build-env
RUN apk --no-cache add build-base git gcc
WORKDIR /src
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY . ./

# ARG needs to come after FROM
ARG LDFLAGS="-w -s"
RUN CGO_ENABLED=0 go build -v -o radius-rp -ldflags "$LD_FLAGS" cmd/rp/main.go

FROM alpine
RUN apk --no-cache add ca-certificates
COPY --from=build-env /src/radius-rp ./radius-rp
EXPOSE 5000
ENTRYPOINT ./radius-rp