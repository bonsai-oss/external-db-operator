FROM golang:alpine@sha256:feceecc0e1d73d085040a8844de11a2858ba4a0c58c16a672f1736daecc2a4ff AS builder
WORKDIR /build
COPY . .
ENV CGO_ENABLED=0
RUN apk add --no-cache ca-certificates
RUN go build -trimpath -ldflags '-s -w' -o /bin/operator main.go

FROM scratch
COPY --from=builder /bin/operator /operator
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/operator"]