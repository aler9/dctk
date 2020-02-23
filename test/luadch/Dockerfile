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
    && rm -rf /var/lib/apt/lists/*

RUN git clone --depth=1 -b v2.20 https://github.com/luadch/luadch \
    && cd /luadch \
    && ./compile \
    && mv ./build_gcc/luadch /luadch-out \
    && rm -rf /luadch

WORKDIR /luadch-out

# allow guests
RUN sed -i "s/\(reg_only\) = .\\+,/\1 = false,/" ./cfg/cfg.tbl

# disable redirect
RUN sed -i "s/\(cmd_redirect_activate\) = .\\+,/\1 = false,/" ./cfg/cfg.tbl

# do not use tags
RUN sed -i "s/\(usr_nick_prefix_activate\) = .\\+,/\1 = false,/" ./cfg/cfg.tbl

# do not limit traffic
RUN sed -i "s/\(etc_trafficmanager_activate\) = .\\+,/\1 = false,/" ./cfg/cfg.tbl

# activate tls
RUN cd ./certs && echo "testing" > UID.txt && ./make_cert.sh

RUN sed -i 's/test/testpa$ss/' ./cfg/user.tbl
RUN sed -i "s/dummy/testdctk_auth/" ./cfg/user.tbl

#RUN cat ./cfg/cfg.tbl; exit 1
#RUN cat ./cfg/user.tbl; exit 1

ENTRYPOINT [ "./luadch" ]
