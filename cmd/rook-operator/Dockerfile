FROM alpine:3.4
RUN apk add --no-cache ca-certificates

COPY rook-operator /opt/rook/bin
RUN chmod +x /opt/rook/bin/rook-operator

ENTRYPOINT ["/opt/rook/bin/rook-operator"]
