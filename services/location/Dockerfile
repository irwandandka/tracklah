FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o location .

FROM alpine:3.20 AS production
WORKDIR /app
COPY --from=build /app/location .
# Run as an unprivileged user, not root.
RUN adduser -D -H -u 10001 app
USER 10001
EXPOSE 8080
CMD ["./location"]
