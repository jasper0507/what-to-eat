# syntax=docker/dockerfile:1

FROM node:24-alpine AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/what-to-eat ./cmd/server

FROM alpine:3.23
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=api /out/what-to-eat /usr/local/bin/what-to-eat
COPY --from=web /src/frontend/dist ./frontend/dist
RUN mkdir data && chown app:app data
USER app
ENV APP_ENV=production \
    DATABASE_PATH=/app/data/what-to-eat.db \
    WEB_DIR=/app/frontend/dist \
    PORT=8080
EXPOSE 8080
VOLUME ["/app/data"]
CMD ["what-to-eat"]
