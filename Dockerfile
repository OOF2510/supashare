FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk --no-cache add ffmpeg upx

COPY go.mod go.sum ./
RUN go mod download
RUN go mod tidy


COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o main .
RUN upx --best --lzma ./main

FROM alpine:3.19

RUN apk --no-cache add ca-certificates ffmpeg

WORKDIR /app

COPY ./pages/ ./pages/

COPY --from=builder /app/main .

EXPOSE 8080

CMD ["./main"]
