FROM golang:1.23.3 AS builder

LABEL maintainer="darmiel <hi@d2a.io>"
LABEL org.opencontainers.image.source = "https://github.com/darmiel/twitch-chatlog"

WORKDIR /usr/src/app
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# Install and cache dependencies (by @montanaflynn)
# https://github.com/montanaflynn/golang-docker-cache
COPY go.mod go.sum ./
RUN go mod graph | awk '{if ($1 !~ "@") print $2}' | xargs go get

# Copy remaining source
COPY *.go .
COPY go.mod .
COPY go.sum .

RUN GOOS=linux GOARCH=amd64 go build -o chatlog .

FROM alpine:3

ADD https://github.com/golang/go/raw/master/lib/time/zoneinfo.zip /zoneinfo.zip
ENV ZONEINFO /zoneinfo.zip

RUN addgroup -S nonroot \
    && adduser -S nonroot -G nonroot \
    && chown nonroot:nonroot /zoneinfo.zip

USER nonroot

COPY --from=builder /usr/src/app/chatlog .

EXPOSE 80

ENTRYPOINT [ "/chatlog" ]