# MeetGo — Dating App Server

Backend для приложения знакомств на Go. На текущем этапе — голый рабочий каркас
(HTTP-сервер, подключение к PostgreSQL, инфраструктура миграций, Docker). Доменные
сущности будут добавлены позже.

## Стек

- **Go 1.26**
- HTTP: `net/http` + [`chi`](https://github.com/go-chi/chi)
- БД: **PostgreSQL** + [`pgx`](https://github.com/jackc/pgx) (pgxpool)
- SQL → код: [`sqlc`](https://sqlc.dev)
- Миграции: [`golang-migrate`](https://github.com/golang-migrate/migrate)
- API-документация: [`swaggo/swag`](https://github.com/swaggo/swag) (Swagger UI)
- Логи: stdlib `log/slog` (structured)

## Структура

```
cmd/server        — точка входа
internal/config   — загрузка конфигурации из ENV
internal/database — пул соединений к PostgreSQL
internal/server   — http.Server, роутер, middleware, graceful shutdown
internal/handler  — HTTP-обработчики (пока health/ready)
internal/db       — sqlc: queries/ и сгенерированный sqlc/ (появятся позже)
migrations        — SQL-миграции (golang-migrate)
deployments       — docker-compose
```

## Требования

- Go 1.26+
- Docker + Docker Compose
- CLI-инструменты (ставятся отдельно):
  ```bash
  go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
  go install github.com/swaggo/swag/cmd/swag@latest
  ```

## Запуск

```bash
cp .env.example .env      # прописать DATABASE_URL и JWT_SECRET

# Вариант 1: локальный PostgreSQL (без Docker)
#   1. создать пустую БД один раз:  CREATE DATABASE meetgo;
#   2. указать свой DATABASE_URL в .env
make migrate-up           # применить миграции (создаёт таблицы)
make run                  # ENV=dev → OTP-код всегда 000000
# или с авто-перезапуском при изменении файлов (как nodemon):
make dev                  # требует air: go install github.com/air-verse/air@latest

# Вариант 2: всё в Docker
make docker-up            # postgres сам создаёт БД meetgo
make migrate-up
```

> Миграции создают **таблицы**, но не саму БД — пустую базу нужно создать заранее
> (в Docker это делает контейнер автоматически).

Проверка:

```bash
curl localhost:8080/healthz   # 200 ok       (liveness)
curl localhost:8080/readyz    # 200 ready     (readiness, пингует БД)
```

Swagger UI: <http://localhost:8080/swagger/index.html>
(спецификация — `/swagger/doc.json`).

## Полезные команды

```bash
make help          # список целей
make build         # собрать бинарь в ./bin
make tidy          # go mod tidy
make vet           # go vet ./...
make migrate-create name=add_users
make sqlc          # генерация кода из SQL
make swag          # регенерация Swagger-доков в internal/docs
```

## Endpoints

| Метод | Путь        | Назначение            |
|-------|-------------|-----------------------|
| GET   | `/healthz`  | liveness probe        |
| GET   | `/readyz`   | readiness probe (БД)  |
| GET   | `/swagger/*`| Swagger UI / OpenAPI  |

### Auth (`/api/v1`)

| Метод | Путь                  | Bearer | Назначение                          |
|-------|-----------------------|--------|-------------------------------------|
| POST  | `/auth/send_code`     | —      | отправить OTP на email              |
| POST  | `/auth/check_code`    | —      | проверить OTP, выдать токены        |
| POST  | `/auth/refresh`       | —      | ротация пары токенов                |
| POST  | `/auth/logout`        | ✓      | ревок текущей сессии                |
| GET   | `/me`                 | ✓      | идентичность аккаунта               |

### Профиль / онбординг (`/api/v1`, Bearer)

| Метод | Путь                    | Назначение                                  |
|-------|-------------------------|---------------------------------------------|
| GET   | `/interests`            | справочник интересов `[{id,value}]`         |
| GET   | `/me/profile`           | профиль (404 `PROFILE_NOT_FOUND`)           |
| PUT   | `/me/profile/basics`    | шаг 1: имя, пол, дата рождения (18+), город  |
| PUT   | `/me/profile/about`     | шаг 2: интересы(1–5), описание(30–1000), цель, рост/вес |
| POST  | `/me/profile/photos`    | шаг 3: загрузка фото (multipart `photo`)    |
| PATCH | `/me/profile/photos/{id}`| crop фото (главное)                         |
| PATCH | `/me/profile/photos/order`| порядок (первый = главное)                 |
| DELETE| `/me/profile/photos/{id}`| удалить фото                                |
| POST  | `/me/profile/complete`  | финал онбординга (≥2 фото) → `DONE`         |

Шаги последовательные: `about` до `basics` → `409 STEP_ORDER`. Прогресс —
`onboardingStep` ∈ `BASICS`→`ABOUT`→`PHOTOS`→`DONE`. Фото: jpeg/png/webp, ≤10 МБ,
до 5 шт, минимум 2 на `complete`.

### Хранилище фото (DI по ENV)

Единый интерфейс `Storage`, провайдер выбирается `STORAGE_PROVIDER`:
- `local` (по умолчанию, dev) — файлы в `STORAGE_LOCAL_DIR` (`./uploads`),
  отдаются статикой на `STORAGE_PUBLIC_URL` (`/uploads`); 0 доп. процессов.
- `s3` (prod) — любой S3-совместимый (MinIO/AWS S3/Cloudflare R2) через `minio-go`.

Код модуля фото зависит только от интерфейса — провайдер инжектится в `main`.

### GeoIP и правило «русская почта для РФ»

Если клиентский IP по GeoIP определяется как **RU**, на `send_code` требуется
**российская почта** (`.ru`/`.su`/`.рф`/punycode `.xn--p1ai`), иначе
`422 RU_EMAIL_REQUIRED`. Для не-RU и при выключенном GeoIP ограничения нет
(блокировок по стране нет — это мягкое правило, VPN-обход допустим).

- База — локальный `.mmdb` (offline, без внешних запросов). Путь — `GEOIP_DB_PATH`;
  пусто → GeoIP выключен, правило не срабатывает.
- Где взять базу: **MaxMind GeoLite2 Country** (бесплатно, нужен аккаунт+ключ) или
  **DB-IP Lite Country** (бесплатно, прямая загрузка). Положить файл и указать путь.
- В `ENV=dev` заголовок **`X-Debug-Country: RU`** переопределяет GeoIP — удобно
  тестировать правило и симулировать регион с клиента.

В dev-режиме (`ENV=dev`) OTP-код всегда **`000000`** (реальная почта не шлётся, код логируется).
