FROM amd64/golang:1.12-stretch

WORKDIR /

RUN git clone https://github.com/aler9/go-dcpp \
    && cd /go-dcpp \
    && go install ./cmd/go-hub \
    && rm -rf /go-dcpp

RUN go-hub init

ENTRYPOINT [ "go-hub", "serve" ]
