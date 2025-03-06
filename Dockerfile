FROM golang:1.24 AS builder
COPY . /src
WORKDIR /src
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM alpine:3.21.3 AS runner
RUN apk add --no-cache tzdata
ENV TZ=Australia/Adelaide

WORKDIR /app
COPY --from=builder /src/main .

EXPOSE 8080

ENTRYPOINT ["./main", "winet-controller"]

