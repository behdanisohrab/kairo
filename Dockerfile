# syntax=docker/dockerfile:1

FROM golang:1.21-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=0.2.0
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /kairo .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /kairo /usr/local/bin/kairo
WORKDIR /data
EXPOSE 53/udp 53/tcp 853/tcp 443/tcp 8080/tcp
ENTRYPOINT ["/usr/local/bin/kairo"]
CMD ["-config", "/data/config.json"]
