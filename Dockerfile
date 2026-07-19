FROM golang:1.26.2-alpine AS BUILD

RUN apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY . .

# CGO_ENABLED=0 для автономности бинаря в alpine версии
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o backend ./cmd/url-shortener/main.go

FROM alpine:3.20 as FINAL

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=BUILD /build/backend .
COPY --from=BUILD /build/migrations /app/migrations

CMD ["./backend"]