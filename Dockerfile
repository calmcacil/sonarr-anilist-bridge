FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /server /server

EXPOSE 8080

VOLUME ["/data"]

ENTRYPOINT ["/server"]
