FROM ubuntu
MAINTAINER Jared Watts <jared.watts@quantum.com>

ENV ROOK_VERSION 0.2.1

RUN apt-get update && \
    apt-get install -yq --no-install-recommends wget s3cmd kmod module-init-tools sudo

RUN wget --no-check-certificate -P /tmp https://github.com/rook/rook/releases/download/v${ROOK_VERSION}/rook-v${ROOK_VERSION}-linux-amd64.tar.gz && \
    gunzip /tmp/rook-v${ROOK_VERSION}-linux-amd64.tar.gz && \
    tar -xf /tmp/rook-v${ROOK_VERSION}-linux-amd64.tar -C /tmp && \
    mv /tmp/rook /usr/local/bin/ && \
    chmod 0755 /usr/local/bin/rook && \
    rm -rf /tmp/rook*


