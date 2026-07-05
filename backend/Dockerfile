FROM golang:1.22-alpine AS builder

WORKDIR /app
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# go.sum in the archive may be outdated/empty; tidy after copying sources so Docker can resolve all imported modules.
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/invest-api ./cmd/api

FROM alpine:3.20

WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/invest-api /app/invest-api

EXPOSE 8080
CMD ["/app/invest-api"]
