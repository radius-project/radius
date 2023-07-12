# Issues on M1 mac: https://github.com/docker/for-mac/issues/5831
# it is a multistage docker creation for small magpie image
FROM amd64/golang:1.20-alpine3.18
WORKDIR /usr/src/tmpapp

# Download necessary Go modules
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY bindings ./bindings
COPY *.go ./
# create the go binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o magpiego .

FROM amd64/alpine:3.18
WORKDIR /usr/src/app
COPY --from=0 /usr/src/tmpapp/magpiego ./
EXPOSE 3000

#command to start the magpie as a server to listen at port 3000
CMD [ "./magpiego" ]