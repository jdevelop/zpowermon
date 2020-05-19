FROM golang:1.14-alpine AS builder
COPY . /build/
WORKDIR /build
ARG VER=LATEST
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=${VER}" -trimpath -o /build/zpowermon ./app/

FROM alpine:3.10
COPY --from=builder /build/zpowermon /opt/
WORKDIR /opt
ENTRYPOINT ["/opt/zpowermon"]
