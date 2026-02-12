ARG GO_VERSION=1.24.0
FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
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
    psql "postgresql://${DB_TASKS_USERNAME}:${DB_TASKS_PASSWORD}@${DB_TASKS_HOST}:${DB_TASKS_PORT}/${DB_TASKS_DATABASE}" \
         -f /app/migrations/0001_create_enums.up.sql && \
    psql "postgresql://${DB_TASKS_USERNAME}:${DB_TASKS_PASSWORD}@${DB_TASKS_HOST}:${DB_TASKS_PORT}/${DB_TASKS_DATABASE}" \
         -f /app/migrations/0002_create_tags_tables.up.sql && \
    psql "postgresql://${DB_TASKS_USERNAME}:${DB_TASKS_PASSWORD}@${DB_TASKS_HOST}:${DB_TASKS_PORT}/${DB_TASKS_DATABASE}" \
         -f /app/migrations/0003_create_tasks_table.up.sql && \
    psql "postgresql://${DB_TASKS_USERNAME}:${DB_TASKS_PASSWORD}@${DB_TASKS_HOST}:${DB_TASKS_PORT}/${DB_TASKS_DATABASE}" \
         -f /app/migrations/0004_seed_default_tags.up.sql && \
    psql "postgresql://${DB_TASKS_USERNAME}:${DB_TASKS_PASSWORD}@${DB_TASKS_HOST}:${DB_TASKS_PORT}/${DB_TASKS_DATABASE}" \
         -f /app/migrations/0005_create_outbox_table.up.sql && \
    echo "Миграции завершены, запускаем приложение..." && \
    ./tasks-service