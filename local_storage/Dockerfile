FROM golang:1.25.1-alpine3.22

WORKDIR /go/src/storage
ADD go.mod go.mod
ADD go.sum go.sum
RUN go mod download
ADD main.go main.go
RUN go build -o storage
CMD ["./storage"]
