FROM ubuntu:xenial
RUN apt-key adv --keyserver hkp://p80.pool.sks-keyservers.net:80 --recv-keys 08b73419ac32b4e966c1a330e84ac2c0460f3994 \
    && echo "deb http://download.ceph.com/debian-${CEPH_BRANCH}/ xenial main" > /etc/apt/sources.list.d/ceph-${CEPH_BRANCH}.list \
    && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -yq --no-install-recommends \
        binutils \
        ceph-common \
        s3cmd \
        curl \
        fio \
        gdb \
        iperf3 \
        jq \
        kmod \
        less \
        man-db \
        sudo \
        vim \
    && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
COPY root /
COPY entrypoint.sh /toolbox/
ENTRYPOINT [ "/toolbox/entrypoint.sh" ]
