# FROM amd64/golang:1.18.3-alpine3.16
# WORKDIR /usr/src/app


# # COPY go.mod ./
# # COPY go.sum ./
# # RUN go mod 
# # COPY bindings ./bindings
# COPY bookstore ./osm_demo_bookstore
# # COPY *.go ./
# RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bookstore .


FROM amd64/golang:1.18.3-alpine3.16
WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download
# COPY bindings ./bindings
COPY *.go ./
# CMD ["/bookstore"]
RUN GOOS=linux GOARCH=amd64 go build -o ./bookstore .


FROM amd64/alpine:3.16
WORKDIR /app
EXPOSE 14001
ARG port=14001
COPY --from=0 ./app/bookstore ./
# RUN make /app
