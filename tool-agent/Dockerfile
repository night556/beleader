FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o tool-agent .

FROM alpine:latest
RUN apk --no-cache add ca-certificates bash
WORKDIR /app
COPY --from=builder /app/tool-agent .
EXPOSE 8083
CMD ["./tool-agent"]
