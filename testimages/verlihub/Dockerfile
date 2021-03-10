FROM amd64/debian:stretch-slim

ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    g++ \
    make \
    cmake \
    libpcre3-dev \
    libssl-dev \
    libmariadbclient-dev-compat \
    mariadb-client \
    libmaxminddb-dev \
    libmaxminddb0 \
    libicu-dev \
    gettext \
    libasprintf-dev \
    mariadb-server \
    netcat-traditional \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --depth=1 -b 1.2.0.0 https://github.com/Verlihub/verlihub \
    && cd /verlihub \
    && cmake . \
    && make -j$(nproc) \
    && make install \
    && ldconfig \
    && rm -rf /verlihub*

COPY setup.sh /
RUN chmod +x /setup.sh \
    && /setup.sh \
    && rm /setup.sh

COPY start.sh /
RUN chmod +x /start.sh

ENTRYPOINT [ "/start.sh" ]
