FROM amd64/golang:1.12-stretch

WORKDIR /

RUN git clone https://github.com/direct-connect/go-dcpp \
    && cd /go-dcpp \
    && git checkout 8bc128e \
    && go install ./cmd/go-hub \
    && rm -rf /go-dcpp

RUN go-hub init

ENTRYPOINT [ "go-hub", "serve" ]
