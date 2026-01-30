FROM golang:1.22-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /out/server /app/server
COPY internal/web /app/internal/web
COPY migrations /app/migrations

EXPOSE 8080

ENTRYPOINT ["/app/server"]
