FROM golang:1.14-alpine

RUN apk --no-cache add make bash fish build-base

WORKDIR /flume

COPY ./Makefile ./go.mod ./go.sum /flume/
RUN make tools

COPY ./ /flume

CMD make all
