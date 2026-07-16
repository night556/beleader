FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/runtime .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /app/runtime /app/runtime
EXPOSE 8083
ENTRYPOINT ["/app/runtime"]
