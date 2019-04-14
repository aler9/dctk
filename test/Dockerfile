FROM amd64/golang:1.11-stretch

ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    netcat-traditional \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

# wait for hub before execution
RUN echo '#!/bin/sh \n\
while true; do \n\
    HOST_IP=${HUBURL##*://} \n\
    nc -z -v -w1 ${HOST_IP%:*} ${HOST_IP##*:} 2>/dev/null && break \n\
done \n\
go run test/$UNIT.go \n\
' > /start.sh && chmod +x /start.sh

ENTRYPOINT [ "/start.sh" ]
