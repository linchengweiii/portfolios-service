# syntax=docker/dockerfile:1

########################
# Build stage (Go 1.24.1)
########################
FROM golang:1.24.1-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates

# Make module downloads more robust
ENV GOPROXY=https://proxy.golang.org,https://goproxy.io,https://goproxy.cn,direct \
    GOSUMDB=sum.golang.org \
    CGO_ENABLED=0

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Optional cross-compile args (defaults are linux/amd64)
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH

RUN go build -trimpath -ldflags "-s -w" -o /out/portfolio-service

########################
# Runtime stage
########################
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S app && adduser -S -G app app

WORKDIR /data
VOLUME ["/data"]

# Defaults: CSV repo + data dir; provide API key at runtime
ENV REPO_KIND=csv \
    DATA_DIR=/data

EXPOSE 8080
COPY --from=build /out/portfolio-service /usr/local/bin/portfolio-service
USER app
ENTRYPOINT ["/usr/local/bin/portfolio-service"]
