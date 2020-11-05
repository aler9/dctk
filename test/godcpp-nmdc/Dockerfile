FROM amd64/golang:1.13-stretch

WORKDIR /

RUN git clone https://github.com/direct-connect/go-dcpp \
    && cd /go-dcpp \
    && git checkout 981f564 \
    && go install ./cmd/go-hub \
    && rm -rf /go-dcpp

RUN go-hub init

ENTRYPOINT [ "go-hub", "serve" ]
