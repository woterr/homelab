# homelab backend — multi-stage build
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-arm64} go build -trimpath -ldflags="-s -w" -o /homelab ./cmd/homelab

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata util-linux-misc
RUN adduser -D -H -u 1000 homelab
COPY --from=builder /homelab /usr/local/bin/homelab
USER homelab
VOLUME /data
EXPOSE 8000
ENTRYPOINT ["/usr/local/bin/homelab"]