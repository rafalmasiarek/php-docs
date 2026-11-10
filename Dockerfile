FROM golang:1.23-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/php-docs .

################

FROM alpine:3.20

RUN apk add --no-cache --upgrade ca-certificates

COPY --from=builder /out/php-docs /usr/local/bin/php-docs

ENTRYPOINT ["php-docs"]
