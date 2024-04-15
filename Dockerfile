FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY . .
ENV CGO_ENABLED=0
RUN apk add --no-cache ca-certificates
RUN go build -trimpath -ldflags '-s -w' -o /bin/operator main.go

FROM scratch
COPY --from=builder /bin/operator /operator
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/operator"]