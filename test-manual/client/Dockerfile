FROM amd64/golang:1.13-alpine3.10

WORKDIR /s

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN go build -o /out ./test-manual/client

COPY test-manual/client/start.sh /
RUN chmod +x /start.sh

ENTRYPOINT [ "/start.sh" ]
