FROM golang:alpine@sha256:a6a7f1fcf12f5efa9e04b1e75020931a616cd707f14f62ab5262bfbe109aa84a AS builder
WORKDIR /build
COPY . .
ENV CGO_ENABLED=0
RUN apk add --no-cache ca-certificates
RUN go build -trimpath -ldflags '-s -w' -o /bin/operator main.go

FROM scratch
COPY --from=builder /bin/operator /operator
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/operator"]