FROM golang:1.24-alpine AS builder
ENV GOTOOLCHAIN=local
ENV GOPROXY=https://proxy.golang.org,direct
ENV GONOSUMCHECK=*
ENV GONOSUMDB=*
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
ARG CACHEBUST=1
COPY . .
# Проверяем что миграции существуют
RUN ls -la /app/app/migrations/ || echo "No migrations found"
RUN CGO_ENABLED=0 GOOS=linux go build -o tasks-service ./app/cmd

FROM alpine:latest
RUN apk --no-cache add ca-certificates postgresql-client
WORKDIR /app
RUN mkdir -p /app/logs && chmod 755 /app/logs
# Копируем миграции
COPY --from=builder /app/app/migrations /app/migrations
# Проверяем что скопировались
RUN ls -la /app/migrations/
COPY --from=builder /app/tasks-service .
EXPOSE 8000
CMD echo "Запускаем миграции..." && \
    DB_URL="postgresql://${DB_TASKS_USERNAME}:${DB_TASKS_PASSWORD}@${DB_TASKS_HOST}:${DB_TASKS_PORT}/${DB_TASKS_DATABASE}" && \
    for f in $(ls /app/migrations/*.up.sql | grep -v '0008_' | sort); do \
        echo "Применяем $f..."; \
        psql "$DB_URL" -f "$f"; \
    done && \
    echo "Миграции завершены, запускаем приложение..." && \
    ./tasks-service