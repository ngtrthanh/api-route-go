FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod main.go ./
RUN go build -o /app

FROM alpine:3.19
WORKDIR /app
COPY --from=build /app .
COPY *.csv .
CMD ["./app"]
