FROM alpine:3.14

VOLUME /go/src/github.com/rook/rook

RUN apk add --no-cache \
    go \
    python3 \
    python3-dev \
    py3-pip

RUN pip3 install -U pip setuptools wheel
RUN pip3 install ruamel.yaml

COPY extract-rbac.py .
