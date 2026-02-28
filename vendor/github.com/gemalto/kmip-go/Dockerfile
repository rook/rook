FROM golang:alpine

RUN apk --no-cache add make git curl bash fish

WORKDIR /project

COPY ./ /project
RUN make tools

CMD make
