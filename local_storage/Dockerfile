FROM gcr.io/shibuya-214807/golang:1.13.6-stretch

WORKDIR /go/src/storage
ADD go.mod go.mod
ADD go.sum go.sum
RUN go mod download
ADD main.go main.go
RUN go build -o storage
CMD ["./storage"]
