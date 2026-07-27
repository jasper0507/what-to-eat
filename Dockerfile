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

# HowToCook 菜谱图走 Git LFS。GitHub archive/tarball 只含 pointer 文本
# （~130B 的 ASCII，Content-Type 仍像 image/*），浏览器解码失败 → 菜谱页无图。
# 必须 clone + git lfs pull 才能装进真实图片。
FROM alpine:3.23 AS catalog
ARG HOWTOCOOK_REF=c05758fa661ac4efa0361a987b700a351a22159b
RUN apk add --no-cache git git-lfs \
    && git lfs install \
    && git clone --filter=blob:none --no-checkout \
         https://github.com/Anduin2017/HowToCook.git /tmp/HowToCook \
    && cd /tmp/HowToCook \
    && git sparse-checkout set dishes LICENSE \
    && git checkout "$HOWTOCOOK_REF" \
    && git lfs pull \
    && mv dishes /catalog \
    && mv LICENSE /HOWTOCOOK-LICENSE \
    && rm -rf /tmp/HowToCook \
    && if grep -RIl \
          --include='*.jpg' --include='*.jpeg' --include='*.png' \
          --include='*.webp' --include='*.bmp' --include='*.JPG' \
          'git-lfs.github.com' /catalog >/dev/null; then \
         echo "catalog still contains Git LFS pointer files" >&2; \
         exit 1; \
       fi

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
