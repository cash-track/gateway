FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN go build -v -o gateway

FROM alpine:3.18

RUN apk add ca-certificates
WORKDIR /app
COPY --from=builder /app/gateway /app/gateway
EXPOSE 80
CMD ["/app/gateway"]
