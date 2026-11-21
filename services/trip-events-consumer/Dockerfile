FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o trip-events-consumer .

FROM alpine:3.20 AS production
WORKDIR /app
COPY --from=build /app/trip-events-consumer .
# Run as an unprivileged user, not root.
RUN adduser -D -H -u 10001 app
USER 10001
CMD ["./trip-events-consumer"]
