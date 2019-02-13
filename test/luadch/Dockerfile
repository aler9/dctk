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

# allow guests
RUN sed -i "s/\(reg_only\) = .\\+,/\1 = false,/" /luadch-out/cfg/cfg.tbl

# disable redirect
RUN sed -i "s/\(cmd_redirect_activate\) = .\\+,/\1 = false,/" /luadch-out/cfg/cfg.tbl

# do not use tags
RUN sed -i "s/\(usr_nick_prefix_activate\) = .\\+,/\1 = false,/" /luadch-out/cfg/cfg.tbl

# do not limit traffic
RUN sed -i "s/\(etc_trafficmanager_activate\) = .\\+,/\1 = false,/" /luadch-out/cfg/cfg.tbl

RUN sed -i 's/test/testpa$ss/' /luadch-out/cfg/user.tbl
RUN sed -i "s/dummy/testdctk_auth/" /luadch-out/cfg/user.tbl

#RUN cat /luadch-out/cfg/cfg.tbl; exit 1
#RUN cat /luadch-out/cfg/user.tbl; exit 1

ENTRYPOINT [ "./luadch" ]
