# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X kairo/internal/version.Version=${VERSION}" -o /kairo ./cmd/kairo

FROM gcr.io/distroless/static-debian12:latest
COPY --from=build /kairo /usr/local/bin/kairo
ARG VERSION=dev
ENV KAIRO_VERSION=$VERSION
WORKDIR /data
EXPOSE 53/udp 53/tcp 853/tcp 443/tcp 8080/tcp
ENTRYPOINT ["/usr/local/bin/kairo"]
CMD ["run", "--config", "/data/config.yaml"]
