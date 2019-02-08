#!/bin/sh

docker build . \
    -f ./example/Dockerfile \
    -t dctk-example \
    && docker run --rm -it \
    -v $PWD:/src \
    -p 3009:3009 \
    -p 3009:3009/udp \
    -p 3010:3010 \
    dctk-example $@
