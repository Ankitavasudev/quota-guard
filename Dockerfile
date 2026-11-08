# syntax=docker/dockerfile:1

FROM golang:1.25 AS builder
WORKDIR /src

# Cache dependencies first.
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/quota-guard ./cmd/quota-guard

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/quota-guard /quota-guard
USER 65532:65532
ENTRYPOINT ["/quota-guard"]
