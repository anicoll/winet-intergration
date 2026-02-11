FROM golang:1.26 AS builder
COPY . /src
WORKDIR /src
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM alpine:latest AS runner
RUN apk add --no-cache tzdata curl
ENV TZ=Australia/Adelaide

WORKDIR /app
COPY --from=builder /src/main .
COPY ./migrations  /app/migrations

EXPOSE 8080

ENTRYPOINT ["./main", "winet-controller"]

