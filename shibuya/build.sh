#!/bin/bash

target=$1
mkdir -p build
export GO111MODULE=on
go mod download

case "$target" in
    "jmeter") GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o build/shibuya-agent $(pwd)/engines/jmeter
    ;;
    "controller") GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o build/shibuya-controller $(pwd)/controller/cmd
    ;;
    "coordinator") GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o build/shibuya-coordinator $(pwd)/coordinator
    ;;
    "local_storage") GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o build/shibuya-local-storage $(pwd)/local_storage
    ;;
    *)
    GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o build/shibuya
esac
