#!/bin/sh

docker build . \
    -f ./example/Dockerfile \
    -t dctk-example \
    && docker run --rm -it \
    -v $PWD:/src \
    -e 3006 \
    -e 3006/udp \
    -p 3006:3006 \
    -p 3006:3006/udp \
    dctk-example $@
