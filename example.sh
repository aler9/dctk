#!/bin/sh

[ -z "$@" ] && { echo "usage: $0 [example_file]" 2>&1; exit 1; }

docker run --rm -it \
    -v $PWD:/src \
    -p 3009:3009 \
    -p 3009:3009/udp \
    -p 3010:3010 \
    amd64/golang:1.11-stretch bash -c "\
    cd /src \
    && go run example/$1.go"
