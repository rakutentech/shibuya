#!/bin/bash

target=$1
mkdir -p build
export GO111MODULE=on
go mod download

case "$target" in
    "jmeter") GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o build/shibuya-agent $(pwd)/engines/jmeter
    ;;
    *)
    GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o build/shibuya
esac