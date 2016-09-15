FROM ubuntu:xenial
MAINTAINER Bassam Tabbara <bassam.tabbara@quantum.com>

# git version 2.6.6+ through 2.8.3 had a bug with submodules
# see https://github.com/git/git/blob/master/Documentation/RelNotes/2.8.3.txt#L33
# this is why we are adding a newer verion of git than is available on xenial

RUN apt-key adv --keyserver keyserver.ubuntu.com --recv-keys E1DF1F24 && \
    echo "deb http://ppa.launchpad.net/git-core/ppa/ubuntu xenial main" > /etc/apt/sources.list.d/git.list && \
    apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -yq \
    build-essential \
    ca-certificates \
    ccache \
    cmake \
    curl \
    cython \
    cython3 \
    git \
    golang-1.6 \
    libaio-dev \
    libatomic-ops-dev \
    libblkid-dev \
    libboost-date-time-dev \
    libboost-dev \
    libboost-iostreams-dev \
    libboost-program-options-dev \
    libboost-random-dev \
    libboost-regex-dev \
    libboost-system-dev \
    libboost-thread-dev \
    libcrypto++-dev \
    libcurl4-openssl-dev \
    libexpat1-dev \
    libfcgi-dev \
    libgoogle-perftools-dev \
    libjemalloc-dev \
    libkeyutils-dev \
    libldap2-dev \
    libleveldb-dev \
    libsnappy-dev \
    libssl-dev \
    libudev-dev \
    mercurial \
    python-sphinx \
    python-dev \
    python3-all-dev \
    xfslibs-dev \
    yasm \
    --no-install-recommends && \
    apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/lib/go-1.6/bin:$PATH
RUN mkdir -p "$GOPATH/src" "$GOPATH/bin" && chmod -R 777 "$GOPATH"

VOLUME $GOPATH/src/github.com/quantum/castle
WORKDIR $GOPATH/src/github.com/quantum/castle

ENTRYPOINT [ "/bin/bash", "-c" ]
