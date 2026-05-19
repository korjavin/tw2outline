# Stage 1: Build
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o tw2outline .

# Stage 2: Minimal runtime image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/tw2outline .

RUN mkdir -p /app/data

ENV CACHE_FILE_PATH=/app/data/cache.json
ENV TOKEN_FILE_PATH=/app/data/token.json
ENV LOG_LEVEL=INFO
ENV TWITTER_REDIRECT_URL=http://localhost:8080/callback

EXPOSE 8080

CMD ["./tw2outline"]
