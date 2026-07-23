# Build
FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/agent-hub ./cmd/agent-hub

# Run
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget \
  && adduser -D -H -u 10001 app \
  && mkdir -p /data \
  && chown app:app /data
USER app
WORKDIR /app
COPY --from=build /out/agent-hub /app/agent-hub
ENV HTTP_ADDR=:8080
ENV DATA_DIR=/data
EXPOSE 8080
ENTRYPOINT ["/app/agent-hub"]
