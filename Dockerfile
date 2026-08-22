# syntax=docker/dockerfile:1

# Stage 1: Build frontend
FROM docker.io/oven/bun:1-alpine AS frontend
WORKDIR /web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ .
RUN bun run build

# Stage 2: Build Go binary
FROM docker.io/library/golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X kairo/internal/version.Version=${VERSION}" -o /kairo ./cmd/kairo

# Stage 3: Final image
FROM docker.io/library/alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /kairo /usr/local/bin/kairo
COPY --from=frontend /web/dist /app/web/dist
ARG VERSION=dev
ENV KAIRO_VERSION=$VERSION
WORKDIR /data
EXPOSE 53/udp 53/tcp 853/tcp 443/tcp 8080/tcp
ENTRYPOINT ["/usr/local/bin/kairo"]
CMD ["run", "--config", "/data/config.yaml"]
