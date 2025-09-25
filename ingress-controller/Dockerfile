FROM gcr.io/shibuya-214807/golang:1.25.1-alpine3.22 AS builder

# Install git for go mod download
RUN apk update && apk add --no-cache git

WORKDIR /go/src/shibuya-ingress-controller

ENV GO111MODULE=on
ADD go.mod .
ADD go.sum .
RUN go mod download

COPY . /go/src/shibuya-ingress-controller

RUN GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/sic

FROM gcr.io/shibuya-214807/ubuntu:24.04

COPY --from=builder /go/bin/sic /usr/local/bin/sic

CMD ["sic"]
