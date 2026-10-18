# Бэкенд витрины МГУ (Go)

Новый бэкенд на Go вместо `artifacts/api-server` (Express). Главная часть — модуль **Registry**:
реестр элементов витрины.

Registry отвечает только за метаданные карточки:

| Вопрос | Поля |
|---|---|
| что показать, как назвать и описать | `name`, `description`, `icon` |
| куда ведёт элемент | `target_type`, `target` |
| кому его показывать | `visibility`, `allowed_roles` |
| в каком он состоянии | `status` |
| кто за него отвечает | `owner.name`, `owner.contact` |
| порядок на витрине | `sort_order`, `pinned` |

Бизнес-логики конкретных сервисов здесь нет и быть не должно.

## Быстрый старт

Нужен Go 1.22+ (https://go.dev/dl/).

**Без базы данных** (данные берутся из `seed/services.json` и хранятся в памяти):

```powershell
cd backend
$env:STORAGE="memory"; $env:ADMIN_TOKEN="dev-admin-token-change-me"; $env:AUTH_DEV_HEADERS="true"
(export STORAGE="memory"; export ADMIN_TOKEN="dev-admin-token-change-me"; export AUTH_DEV_HEADERS="true") #Linux
go run ./cmd/server
```

**С Postgres** через Docker:

```powershell
cd backend
docker compose up --build -d
docker compose run --rm api seed
```

**С вашим Postgres:**

```powershell
$env:DATABASE_URL="postgres://mgu:mgu@localhost:5432/mgu?sslmode=disable"
export DATABASE_URL="postgres://mgu:mgu@localhost:5432/mgu?sslmode=disable"#Linux
go run ./cmd/server migrate     # миграции (также выполняются при старте, MIGRATE_ON_START=true)
go run ./cmd/server seed        # начальные 22 записи; повторный запуск ничего не дублирует
go run ./cmd/server             # HTTP API на :8081
```

Проверка: `curl http://localhost:8081/api/v1/services`

Переменные окружения описаны в `.env.example`. Приложение не читает `.env` само — задайте
переменные в окружении.

## API

Полная спецификация: [`api/openapi.yaml`](api/openapi.yaml).

| Метод | Путь | Доступ | Описание |
|---|---|---|---|
| GET | `/api/v1/services` | все | витрина для текущего пользователя |
| GET | `/api/v1/services?scope=all&status=draft,active` | админ | весь реестр с владельцами |
| GET | `/api/v1/services/{id}` | все | элемент (404, если не виден) |
| POST | `/api/v1/services` | админ | создать (по умолчанию `status=draft`) |
| PATCH | `/api/v1/services/{id}` | админ | частичное обновление |
| DELETE | `/api/v1/services/{id}` | админ | удалить |
| GET | `/api/healthz`, `/api/readyz` | все | liveness / readiness (проверка БД) |

Ответ витрины (служебные поля `owner`, `visibility`, `allowed_roles` наружу не отдаются):

```json
{
  "items": [
    {
      "id": "49a2e22a-39e1-4c82-a964-ab55e8c45328",
      "slug": "physics-timetable",
      "name": "Расписание физфака",
      "description": "Расписание занятий на физическом факультете МГУ",
      "icon": "📅",
      "target_type": "external_url",
      "target": "https://physics.msu.ru/rus/study/timetable/",
      "status": "active",
      "pinned": false
    }
  ]
}
```

Создание элемента:

```bash
curl -X POST http://localhost:8081/api/v1/services \
  -H "Authorization: Bearer 2rg213r987231yhrf98237yr7214" \
  -H "Content-Type: application/json" \
  -d '{
    "slug": "library", "name": "Библиотека", "icon": "📚",
    "target_type": "external_url", "target": "https://nbmgu.ru/",
    "status": "active", "visibility": "public",
    "owner": {"name": "Команда витрины", "contact": "showcase@example.msu.ru"}
  }'
```

Ошибки всегда возвращаются в одном формате:
`{"error": {"code": "validation_failed", "message": "...", "fields": [{"field": "target", "message": "..."}], "request_id": "..."}}`.

## Правила модели

**Статус**

| Статус | На витрине |
|---|---|
| `draft` | нет (значение по умолчанию при создании через API) |
| `active` | да |
| `maintenance` | да, со `status: "maintenance"` (фронт показывает пометку и/или блокирует переход) |
| `disabled` | нет |

**Видимость**

| `visibility` | Кто видит |
|---|---|
| `public` | все |
| `authenticated` | вошедшие пользователи |
| `role_based` | вошедшие пользователи, у которых есть хотя бы одна роль из `allowed_roles` (список обязателен и непуст) |

**Типы назначения (`target_type`)**

| Тип | `target` | Состояние |
|---|---|---|
| `external_url` | `https://...` или `http://...` | ✅ реализован: валидация URL, запрет `javascript:` и логина/пароля в URL |
| `internal_route` | `/profile` | предусмотрен моделью и БД, API пока отвечает 422 |
| `application` | id приложения | так же |
| `embedded` | URL для встраивания | так же |

Фронтенд выбирает поведение по `target_type` и не знает о конкретных сервисах:
`external_url` открывается в новой вкладке, `internal_route` — через роутер SPA и т.д.
Элементы с типом, который фронт не знает, он должен пропускать.

**Как добавить новый тип назначения**: реализовать `registry.TargetHandler` (метод `Normalize`)
и зарегистрировать его в `targetHandlers` в `internal/registry/target.go`. Схему БД менять не нужно.

**Владелец (`owner`)** обязателен у каждого элемента. Пользователю витрины он не отдаётся —
виден только в `scope=all`.

## Авторизация (временная)

Полноценного RBAC/Keycloak пока нет. Есть точка расширения `auth.Authenticator` и два простых способа:

- `Authorization: Bearer <ADMIN_TOKEN>` — права администратора реестра.
  Если `ADMIN_TOKEN` не задан, запись через API выключена.
- `X-Dev-User: ivan` + `X-Dev-Roles: student,staff` — **только для разработки**
  (`AUTH_DEV_HEADERS=true`), чтобы проверять видимость `authenticated` и `role_based`.

Чтобы подключить Keycloak, нужно добавить `KeycloakAuthenticator`: он проверяет JWT и заполняет
`Principal{Authenticated, Roles, Admin}`, а затем ставится первым в `auth.Chain` в `cmd/server/main.go`.
Registry при этом не меняется.

## Структура

```
backend/
├── cmd/server/main.go          # точка входа: serve | migrate | seed
├── api/openapi.yaml            # контракт API (из него можно генерировать клиент для фронта)
├── internal/
│   ├── registry/               # ДОМЕН: модель, валидация, видимость, сценарии (без HTTP и SQL)
│   │   ├── item.go             #   Item, Status, Visibility, Owner
│   │   ├── target.go           #   TargetType + реестр обработчиков типов назначения
│   │   ├── visibility.go       #   Viewer, правило «кому показывать»
│   │   ├── validate.go         #   нормализация и валидация
│   │   ├── registry.go         #   Catalog / Get / Create / Update / Delete / Seed
│   │   ├── repository.go       #   интерфейс хранилища
│   │   ├── memstore/           #   хранилище в памяти (тесты, STORAGE=memory)
│   │   └── pgstore/            #   хранилище в PostgreSQL
│   ├── db/                     # подключение к Postgres + встроенные SQL-миграции
│   ├── httpapi/                # HTTP: маршруты, DTO, ошибки, middleware (CORS, логи, request-id)
│   ├── auth/                   # кто делает запрос (заглушка → позже Keycloak)
│   ├── seed/                   # загрузка начальных записей из JSON
│   └── config/                 # переменные окружения
└── seed/services.json          # 22 сервиса из старой витрины
```

Зависимости направлены внутрь: `httpapi → registry ← pgstore`. Пакет `registry` не знает
ни про HTTP, ни про SQL.

## Тесты

```powershell
go test ./...
# интеграционный тест с Postgres (ОЧИЩАЕТ таблицу registry_items в указанной БД):
$env:TEST_DATABASE_URL="postgres://mgu:mgu@localhost:5432/mgu_test?sslmode=disable"; go test ./...
export TEST_DATABASE_URL="postgres://mgu:mgu@localhost:5432/mgu_test?sslmode=disable"
```

## Что ещё не сделано

- В `seed/services.json` у всех записей временный владелец «Команда витрины МГУ» —
  его нужно заменить на реальных ответственных.
- Фронтенд (`artifacts/mgu-portal`) пока работает со старым `/api/widgets`. Его нужно
  переключить на `/api/v1/services` и открывать карточки по `target_type`.
- Порядок карточек через drag-and-drop меняется PATCH-запросами `sort_order` по одному
  элементу. Для массовой перестановки позже можно добавить отдельный эндпоинт.
