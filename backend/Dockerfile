# syntax=docker/dockerfile:1
# Multi-stage сборка API. Контекст сборки — корень репозитория (см. compose):
# нужны backend/ и openapi/openapi.yaml (Swagger UI отдаёт контракт из /app).

FROM golang:1.24-alpine AS build

WORKDIR /src

# Сначала только манифесты — слой с зависимостями кешируется.
COPY backend/go.mod backend/go.sum ./backend/
RUN cd backend && go mod download

COPY backend ./backend
COPY openapi ./openapi

RUN cd backend && CGO_ENABLED=0 GOOS=linux go build \
        -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
    && mkdir -p /out/openapi \
    && cp /src/openapi/openapi.yaml /out/openapi/openapi.yaml

FROM alpine:3.21

# ca-certificates — внешние HTTPS (push), tzdata — IANA-пояса для CAL-2,
# wget — HEALTHCHECK.
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S app \
    && adduser -S -G app app

WORKDIR /app

COPY --from=build /out/api /app/api
COPY --from=build /out/openapi/openapi.yaml /app/openapi/openapi.yaml

# Каталог медиа монтируется томом; владелец — непривилегированный пользователь.
RUN mkdir -p /data/uploads && chown -R app:app /app /data

USER app

ENV APP_ENV=production \
    PORT=8080 \
    TZ=Asia/Yekaterinburg \
    OPENAPI_PATH=openapi/openapi.yaml \
    UPLOAD_DIR=/data/uploads

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=6 \
    CMD wget -qO- http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/api"]
CMD ["serve"]
