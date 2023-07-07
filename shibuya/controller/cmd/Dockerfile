FROM gcr.io/shibuya-214807/golang:1.20.2-buster AS builder

WORKDIR /go/src/shibuya-controller

ENV GO111MODULE on
ADD go.mod .
ADD go.sum .
RUN go mod download

COPY . /go/src/shibuya-controller



RUN GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/sc

FROM gcr.io/shibuya-214807/ubuntu:18.04

COPY --from=builder /go/bin/sc /usr/local/bin/sc
COPY config_tmp.json /config.json

CMD ["sc"]
