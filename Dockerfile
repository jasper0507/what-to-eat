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

FROM alpine:3.23 AS catalog
RUN wget -qO /tmp/howtocook.tar.gz \
      https://github.com/Anduin2017/HowToCook/archive/c05758fa661ac4efa0361a987b700a351a22159b.tar.gz \
    && tar -xzf /tmp/howtocook.tar.gz -C /tmp \
    && mv /tmp/HowToCook-c05758fa661ac4efa0361a987b700a351a22159b/dishes /catalog \
    && mv /tmp/HowToCook-c05758fa661ac4efa0361a987b700a351a22159b/LICENSE /HOWTOCOOK-LICENSE

FROM alpine:3.23
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=api /out/what-to-eat /usr/local/bin/what-to-eat
COPY --from=web /src/frontend/dist ./frontend/dist
COPY --from=catalog /catalog ./catalog
COPY --from=catalog /HOWTOCOOK-LICENSE ./licenses/HOWTOCOOK-LICENSE
RUN mkdir data && chown app:app data
USER app
ENV APP_ENV=production \
    DATABASE_PATH=/app/data/what-to-eat.db \
    WEB_DIR=/app/frontend/dist \
    CATALOG_DIR=/app/catalog \
    PORT=8080
EXPOSE 8080
VOLUME ["/app/data"]
CMD ["what-to-eat"]
