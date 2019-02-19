#!/bin/sh

docker build . -f - -t dctk-testcmd << EOF
FROM amd64/golang:1.11-stretch
WORKDIR /src
COPY . ./
RUN go install ./...
EOF

docker run --rm -it \
    dctk-testcmd "$@"
