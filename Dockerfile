FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN go build -v -o gateway

FROM alpine:3.18

ARG GIT_COMMIT
ARG GIT_TAG
ENV GIT_COMMIT=${GIT_COMMIT}
ENV GIT_TAG=${GIT_TAG}

RUN apk add ca-certificates
WORKDIR /app
COPY --from=builder /app/gateway /app/gateway
EXPOSE 80
CMD ["/app/gateway"]
