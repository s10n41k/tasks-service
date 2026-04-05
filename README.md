# tasks-service

Микросервис управления задачами для платформы TODOLIST. Отвечает за CRUD задач, подзадач, тегов, совместных задач и напоминаний.

## Стек

| Слой | Технология |
|------|-----------|
| Язык | Go 1.25 |
| HTTP | httprouter |
| База данных | PostgreSQL (pgx v4) |
| Кэш | Redis |
| Очередь | Kafka |
| Метрики | Prometheus |
| Контейнеризация | Docker |

## Архитектура

Проект следует **Clean Architecture** и **DDD**:

```
app/
├── cmd/                        # Точка входа
├── internal/
│   ├── config/                 # Конфигурация
│   ├── tasks/
│   │   ├── domain/             # Доменные сущности и бизнес-правила
│   │   ├── dto/                # Data Transfer Objects
│   │   ├── port/               # Интерфейсы (репозитории, сервисы)
│   │   ├── service/            # Бизнес-логика
│   │   ├── repository/         # Реализации репозиториев (PostgreSQL, Redis)
│   │   ├── handlers/           # HTTP-хендлеры
│   │   ├── batch/              # Батчевая обработка задач
│   │   ├── notification/       # Клиенты уведомлений
│   │   └── event/kafka/        # Kafka producer
│   ├── shared_tasks/           # Совместные задачи между пользователями
│   ├── tags/                   # Теги задач
│   └── worker/                 # Воркер напоминаний
├── migrations/                 # SQL-миграции
├── pkg/
│   ├── api/                    # Middleware (signature, resilience, sort, filter)
│   ├── client/                 # Клиенты PostgreSQL, Redis, Kafka
│   └── utils/                  # Вспомогательные утилиты
└── tests/e2e/                  # End-to-end тесты
```

## Возможности

- **Задачи** — создание, чтение, обновление, удаление; статусы, приоритеты, дедлайны
- **Подзадачи** — вложенные задачи с автозавершением родительской при выполнении всех подзадач
- **Теги** — категоризация задач
- **Совместные задачи** — предложение задач между пользователями с принятием/отклонением
- **Напоминания** — воркер отправляет уведомления за 60, 15 и 5 минут до дедлайна
- **Батчевая обработка** — оптимизированная запись через batch-воркер
- **Кэширование** — Redis-кэш задач с инвалидацией
- **Outbox pattern** — надёжная доставка событий в Kafka

## Безопасность

Каждый запрос проверяется через **HMAC-signature middleware** — gateway подписывает запросы, сервис верифицирует подпись и timestamp. Запросы старше 30 секунд отклоняются.

## Конфигурация

Сервис конфигурируется через переменные окружения. Пример — см. `.env.example` в infra-репозитории.

Основные переменные:

```env
DB_TASKS_HOST=postgres-tasks
DB_TASKS_PORT=5432
DB_TASKS_DATABASE=tasks
DB_TASKS_USERNAME=tasks_user
DB_TASKS_PASSWORD=...

REDIS_TASKS_ADDR=redis-tasks:6379
REDIS_TASKS_PASSWORD=...

KAFKA_BROKERS=kafka:9092
GATEWAY_SIGN=...
```

## Запуск

Сервис запускается через docker-compose из infra-репозитория:

```bash
# Сборка и запуск
docker-compose up -d tasks-service

# Пересборка без кэша
docker-compose build --no-cache tasks-service && docker-compose up -d tasks-service
```

## Тесты

```bash
# Unit-тесты
go test ./app/internal/tasks/domain/...
go test ./app/internal/tasks/service/...
go test ./app/internal/tasks/dto/...
go test ./app/internal/shared_tasks/...
go test ./app/pkg/api/signature/...

# E2E тесты (требуют запущенный сервис)
go test ./app/tests/e2e/...
```

## CI/CD

- **CI** — запускается при push/PR в `main`: линтер, unit-тесты, сборка Docker-образа
- **CD** — при push в `main` автоматически пересобирает и перезапускает контейнер на self-hosted runner
