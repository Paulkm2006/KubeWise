# ---- Build stage ----
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN GO111MODULE=on GOPROXY=https://goproxy.cn,direct go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /kubewise ./cmd

# ---- Runtime stage ----
FROM alpine:3

RUN apk add --no-cache ca-certificates sqlite-libs

COPY --from=builder /kubewise /usr/local/bin/kubewise

EXPOSE 8080

ENTRYPOINT ["kubewise"]
CMD ["serve"]
