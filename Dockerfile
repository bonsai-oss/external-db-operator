FROM golang:alpine@sha256:5c1cabd9a3c6851a3e18735a2c133fbd8f67fe37eb3203318b7af2ffd2547095 AS builder
WORKDIR /build
COPY . .
ENV CGO_ENABLED=0
RUN apk add --no-cache ca-certificates
RUN go build -trimpath -ldflags '-s -w' -o /bin/operator main.go

FROM scratch
COPY --from=builder /bin/operator /operator
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/operator"]