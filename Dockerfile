FROM golang:1.20-alpine
WORKDIR /app
COPY . .
RUN apk add --no-cache git
ENV CGO_ENABLED=0
CMD ["sh", "-c", "go run ./cmd/manager"]
