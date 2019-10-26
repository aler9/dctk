FROM amd64/debian:stretch-slim

ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    libpcre3-dev \
    libssl-dev \
    mariadb-server \
    libmariadbclient-dev-compat \
    mariadb-client \
    g++ \
    libmaxminddb-dev \
    libmaxminddb0 \
    libicu-dev \
    gettext \
    libasprintf-dev \
    make \
    cmake \
    netcat-traditional \
    && rm -rf /var/lib/apt/lists/*

RUN curl -L https://github.com/Verlihub/verlihub/archive/1.2.0.0.tar.gz | tar xvfz - \
    && cd /verlihub* \
    && cmake . \
    && make -j$(nproc) \
    && make install \
    && ldconfig \
    && rm -rf /verlihub*

COPY setup.sh /
RUN chmod +x /setup.sh
RUN /setup.sh

COPY start.sh /
RUN chmod +x /start.sh

ENTRYPOINT [ "/start.sh" ]
