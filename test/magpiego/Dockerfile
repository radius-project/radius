# Issues on M1 mac: https://github.com/docker/for-mac/issues/5831
FROM golang:1.17.7-alpine
WORKDIR /usr/src/app

# Download necessary Go modules
COPY go.mod ./
COPY go.sum ./
RUN go mod download


COPY bindings ./bindings
COPY *.go ./

# create the go binary
RUN go build -o /magpiego

EXPOSE 3000

#command to start the magpie as a server to listen at port 3000
CMD [ "/magpiego" ]