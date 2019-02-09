FROM amd64/debian:stretch-slim

ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    gcc \
    g++ \
    rsync \
    make \
    libssl-dev \
    netcat-traditional \
    net-tools \
    && rm -rf /var/lib/apt/lists/*

RUN git clone https://github.com/luadch/luadch \
    && cd luadch \
    && git checkout 080fdfe \
    && ./compile \
    && mv ./build_gcc/luadch /luadch-out \
    && cd ../ \
    && rm -rf /luadch

WORKDIR /luadch-out

ENTRYPOINT [ "./luadch" ]
