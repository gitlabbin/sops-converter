# Build the manager binary
FROM golang:1.17 as builder

ARG LDFLAGS
WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY hack/ hack/
COPY pkg/ pkg/

# Build
COPY Makefile Makefile
RUN make generate
RUN CGO_ENABLED=0 GOOS=linux GO111MODULE=on go build -ldflags "$LDFLAGS" -a -o sops-operator main.go

FROM alpine:3.15

RUN apk add --no-cache ca-certificates tzdata bash curl gnupg \
    && addgroup -S --gid 1000 sops-operator \
    && adduser -S -u 1000 -g sops-operator sops-operator

RUN curl -fsSLo /usr/local/bin/sops https://github.com/mozilla/sops/releases/download/v3.7.1/sops-v3.7.1.linux \
    && chmod +x /usr/local/bin/sops

RUN cp /usr/share/zoneinfo/Australia/Sydney /etc/localtime
COPY --from=builder /workspace/sops-operator /usr/local/bin

USER sops-operator
WORKDIR /home/sops-operator
ENTRYPOINT ["sops-operator"]