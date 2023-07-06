# Issues on M1 mac: https://github.com/docker/for-mac/issues/5831
# it is a multistage docker creation for small magpie image
FROM amd64/golang:1.20-alpine3.16
WORKDIR /usr/src/tmpapp

# Download necessary Go modules
COPY go.mod ./
RUN go mod download
COPY *.go ./
# create the go binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o testrp .

FROM amd64/alpine:3.16
WORKDIR /usr/src/app
COPY --from=0 /usr/src/tmpapp/testrp ./
EXPOSE 3000

#command to start the Test RP as a server to listen at port 5000
CMD [ "./testrp" ]
