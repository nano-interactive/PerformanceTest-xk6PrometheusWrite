# Multi-stage build to generate custom k6 with extension
FROM golang:1.19-alpine as builder
WORKDIR /opt/xk6
COPY . .
RUN apk --no-cache add git
RUN go get && go get -u cloud.google.com/go@v0.104.0
RUN go install go.k6.io/xk6/cmd/xk6@v0.7.0
RUN CGO_ENABLED=0 xk6 build \
    --with github.com/nano-interactive/PerformanceTest-xk6PrometheusWrite=. \
    --with github.com/nano-interactive/PerformanceTest-xk6ReadFile \
    --output /tmp/k6

# Create image for running k6 with output for Prometheus Remote Write
FROM alpine:3.15

COPY --from=builder /tmp/k6 /opt/xk6/k6

USER root
WORKDIR /opt/xk6
