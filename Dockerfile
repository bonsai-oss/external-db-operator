FROM golang:alpine@sha256:70afe55365a265f0762257550bc38440e0d6d6b97020d3f8c85328f00200dd8e AS builder
WORKDIR /build
COPY . .
ENV CGO_ENABLED=0
RUN apk add --no-cache ca-certificates
RUN go build -trimpath -ldflags '-s -w' -o /bin/operator main.go

FROM scratch
COPY --from=builder /bin/operator /operator
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/operator"]