#!/bin/sh

docker run --rm -it \
    -v $PWD:/src \
    amd64/golang:1.11-stretch \
    bash -c "cd /src \
    && find . -type f -name '*.go' | xargs gofmt -l -w -s"
