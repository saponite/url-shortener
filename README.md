# URL Shortener

Сервис для сокращения ссылок: принимает длинный URL, возвращает короткий код, по которому позже редиректит на оригинал. Написан на Go, данные в PostgreSQL, маршрутизация на `chi`.

[//]: # (## Содержание)


[//]: # (- [Возможности]&#40;#возможности&#41;)

[//]: # ()
[//]: # (- [Стек]&#40;#стек&#41;)

[//]: # ()
[//]: # (- [Структура проекта]&#40;#структура-проекта&#41;)

[//]: # ()
[//]: # (- [Быстрый старт]&#40;#быстрый-старт&#41;)

[//]: # ()
[//]: # (- [Конфигурация]&#40;#конфигурация&#41;)

[//]: # ()
[//]: # (- [API]&#40;#api&#41;)

[//]: # ()
[//]: # (- [Миграции]&#40;#миграции&#41;)

[//]: # ()
[//]: # (- [Тесты]&#40;#тесты&#41;)

[//]: # ()
[//]: # (- [Деплой через Cloudflare Tunnel]&#40;#деплой-через-cloudflare-tunnel&#41;)

[//]: # ()
[//]: # (- [Roadmap]&#40;#roadmap&#41;)

## Возможности

- Создание коротких ссылок (`POST /create`).
- Редирект с короткого кода на оригинальный URL (`GET /{code}`).
- Дедупликация: повторная отправка той же ссылки возвращает уже существующий код.
- Атомарная вставка через `INSERT ... ON CONFLICT` — без race condition при параллельных запросах.
- Защита от рекурсии: нельзя сократить ссылку, ведущую на собственный домен сервиса.
- Валидация входных URL: схема (`http`/`https`), наличие хоста, валидность домена.
- Конфигурация через `.env` или переменные окружения.
- Готовый `docker-compose.yml` для локального запуска.

## Стек

- **Язык:** Go 1.23+.
- **HTTP-роутер:** [chi](https://github.com/go-chi/chi).
- **БД:** PostgreSQL 16.
- **Драйвер БД:** [jackc/pgx v5](https://github.com/jackc/pgx) (через `pgxpool`).
- **Миграции:** [pressly/goose](https://github.com/pressly/goose).

[//]: # (## Структура проекта)

[//]: # ()
[//]: # (```)

[//]: # (url-shortener/)

[//]: # (├── cmd/)

[//]: # (│   └── url-short/)

[//]: # (│       └── main.go              # точка входа, инициализация пула, роутер)

[//]: # (├── internal/)

[//]: # (│   └── handler/)

[//]: # (│       ├── handler.go           # HTTP-хендлеры и работа с БД)

[//]: # (│       └── ...)

[//]: # (├── migrations/)

[//]: # (│   └── 0001_create_links.sql    # схема таблицы links)

[//]: # (├── web/)

[//]: # (│   └── index.html               # простая UI-страница)

[//]: # (├── .env.example                 # шаблон конфигурации)

[//]: # (├── docker-compose.yml)

[//]: # (├── Dockerfile)

[//]: # (├── go.mod)

[//]: # (├── go.sum)

[//]: # (└── README.md)

[//]: # (```)

## Быстрый старт

### Предварительные требования

- Go 1.23 или новее.
- Docker и Docker Compose.
- `goose` CLI для миграций — `go install github.com/pressly/goose/v3/cmd/goose@latest`. **Опционально.**

Работоспособность проверялась на macOS, Linux.

### 1. Клонировать и настроить окружение

```bash
git clone https://github.com/saponite/url-shortner.git
cd url-shortner
cp .env.example .env
```

По умолчанию `.env` содержит в себе следующую конфигурацию:

```env
DB=postgres
DB_USER=postgres
DB_PASSWORD=12345
DB_HOST=localhost
DB_PORT=5432
DB_NAME=url_shortener
LINK_PREFIX=http://localhost:1234/
```

### 2. Поднять PostgreSQL

```bash
docker compose up -d postgres
```

Проверить, что контейнер создался успешно и уже работает:

```bash
docker compose ps
docker compose logs -f postgres
```

В логах должно появиться `database system is ready to accept connections`.

### 3. Применить миграции

```bash
goose -dir migrations postgres \
  "postgres://postgres:12345@localhost:5432/url_shortener?sslmode=disable" up
```

Если миграция применена, то будет выведено `OK`.

### 4. Запустить сервис

```bash
go run ./cmd/url-shortener
```

или
```bash
go run cmd/urlShortener/main.go 
```

Если запуск произошел успешно, то будет выведена следующая информация:

```
сервер запущен на :1234
```

### 5. Создать первую ссылку

```bash
curl -X POST http://localhost:1234/create \
  -H "Content-Type: application/json" \
  -d '{"original_url": "https://github.com"}'
```

Ответ:

```json
{
  "original_url": "https://github.com",
  "short_url": "http://localhost:1234/abC1xYz",
  "created_at": "2026-04-30T12:00:00Z"
}
```

Открой `short_url` в браузере — будет редирект[*](#деплой-через-cloudflare-tunnel) на GitHub.

## Конфигурация

Все параметры читаются из переменных окружения. Для локального запуска удобно использовать `.env` — он подхватывается автоматически через `godotenv`.

| Переменная     | Назначение                                      | Пример                            |
| -------------- |-------------------------------------------------| --------------------------------- |
| `DB`           | Драйвер БД (схема DSN).                         | `postgres`                        |
| `DB_USER`      | Пользователь БД.                                | `postgres`                        |
| `DB_PASSWORD`  | Пароль.                                         | `12345`                           |
| `DB_HOST`      | Хост БД.                                        | `localhost` (вне compose)         |
| `DB_PORT`      | Порт БД.                                        | `5432`                            |
| `DB_NAME`      | Имя БД.                                         | `url_shortener`                   |
| `LINK_PREFIX`  | Префикс, добавляемый к короткому коду в ответе. | `http://localhost:1234/`          |

> Внутри `docker-compose` сервис обращается к Postgres по имени сервиса. То есть `DB_HOST=postgres`, не `localhost`. Для запуска Go-сервиса вне compose — `DB_HOST=localhost`.


## API

### Создать короткую ссылку

```
POST /create
Content-Type: application/json
```

Запрос:

```json
{
  "original_url": "https://github.com"
}
```

Возможные ответы:

| Статус | Когда                                                                                        |
| ------ |----------------------------------------------------------------------------------------------|
| `201`  | Ссылка создана впервые.                                                                      |
| `200`  | Ссылка уже существовала, возвращён существующий short_code.                                  |
| `400`  | Невалидный JSON, пустой URL, невалидная схема/домен, попытка сократить ссылку на свой домен. |
| `500`  | Внутренняя ошибка (например, БД недоступна).                                                 |

Тело успешного ответа:

```json
{
  "original_url": "https://github.com",
  "short_url": "http://localhost:1234/abC1xYz",
  "created_at": "2026-04-30T12:00:00Z"
}
```

### Редирект по короткой ссылке

```
GET /{code}
```

| Статус | Когда                                                   |
| ------ |---------------------------------------------------------|
| `302`  | Найден код, заголовок `Location` указывает на оригинал. |
| `404`  | Кода нет в БД.                                          |
| `500`  | Внутренняя ошибка.                                      |

С автоматическим переходом:

```bash
curl -L http://localhost:1234/abC1xYz
```

### Главная страница

```
GET /
```

Возвращает простую HTML-страницу из `web/index.html` — форму для создания ссылок.

## Миграции

Схема БД управляется через [goose](https://github.com/pressly/goose). Каждое изменение схемы — отдельный пронумерованный SQL-файл в директории `migrations/`. Goose ведёт служебную таблицу `goose_db_version` и применяет только новые миграции.

### Применить все непримененные

```bash
goose -dir migrations postgres "$DATABASE_URL" up
```

### Откатить последнюю

```bash
goose -dir migrations postgres "$DATABASE_URL" down
```

### Посмотреть статус

```bash
goose -dir migrations postgres "$DATABASE_URL" status
```

### Создать новую миграцию

```bash
goose -dir migrations create add_user_id sql
```

Создастся файл `migrations/<timestamp>_add_user_id.sql` с шаблоном `-- +goose Up` / `-- +goose Down`.

## Тесты

[//]: # (Проект использует трёхуровневый подход к тестированию:)

### Юнит-тесты

Тестируют чистые функции без зависимостей (генерация кода, валидация, преобразование структур).

```bash
go test ./internal/handler -v
```

С детектором гонок (рекомендуется):

```bash
go test ./internal/handler -race -v
```

[//]: # (### Mock-тесты &#40;планируется&#41;)

[//]: # ()
[//]: # (Тестируют логику HTTP-хендлеров с подменой хранилища на интерфейс-мок. Не требуют БД, выполняются миллисекунды. Покрывают краевые случаи &#40;битый JSON, ошибка БД, дубликаты&#41;.)

[//]: # ()
[//]: # (### Интеграционные тесты &#40;планируется&#41;)

[//]: # ()
[//]: # (Поднимают настоящий PostgreSQL через [testcontainers-go]&#40;https://github.com/testcontainers/testcontainers-go&#41; и проверяют полный цикл «HTTP → SQL → ответ».)

[//]: # ()
[//]: # (```bash)

[//]: # (go test ./internal/handler -tags=integration -v)

[//]: # (```)

[//]: # ()
[//]: # (Build-тег `integration` нужен, чтобы интеграционные тесты не запускались в обычном режиме &#40;требуют Docker&#41;.)

## Деплой через Cloudflare Tunnel

Для публикации сервиса в интернет без открытия портов и аренды сервера используется `cloudflared`. Поднимает обратный туннель от твоего компьютера в сеть Cloudflare.

### Быстрый туннель (одноразовый)

Самый простой вариант для демонстрации. Адрес меняется при перезапуске.

```bash
go run ./cmd/url-short  # в одном терминале
cloudflared tunnel --url http://localhost:1234  # в другом
```

В выводе появится URL вида `https://random-words-1234.trycloudflare.com`.

При использовании quick tunnel не забудь обновить `LINK_PREFIX` на полученный домен — иначе в ответах API будут ссылки на `localhost`, которые не работают извне.

### Постоянный туннель со своим доменом

1. Залогиниться в Cloudflare:
   ```bash
   cloudflared tunnel login
   ```

2. Создать туннель:
   ```bash
   cloudflared tunnel create url-shortener
   ```

3. Привязать поддомен:
   ```bash
   cloudflared tunnel route dns url-shortener short.your-domain.com
   ```

4. Создать `~/.cloudflared/config.yml`:
   ```yaml
   tunnel: <UUID-туннеля>
   credentials-file: /home/user/.cloudflared/<UUID>.json

   ingress:
     - hostname: short.your-domain.com
       service: http://localhost:1234
     - service: http_status:404
   ```

5. В `.env` поправить:
   ```env
   LINK_PREFIX=https://short.your-domain.com/
   ```

6. Запустить туннель:
   ```bash
   cloudflared tunnel run url-shortener
   ```

Сервис теперь доступен по `https://short.your-domain.com` с валидным TLS, через CDN Cloudflare.

## Roadmap

- [x] Создание ссылок и редирект
- [x] Дедупликация по `original_url`
- [x] Валидация входных URL
- [x] Конфигурация через `.env`
- [x] Миграции через goose
- [x] Docker Compose
- [ ] Интерфейс `Storage` для разделения слоёв
- [ ] Mock-тесты для хендлеров
- [x] Интеграционные тесты на testcontainers

[//]: # (- [ ] Метрики &#40;Prometheus&#41; и логирование &#40;slog&#41;)

[//]: # (- [ ] Аналитика кликов: счётчик переходов, дата последнего клика)

[//]: # (- [ ] TTL для ссылок &#40;`expires_at`&#41;)

[//]: # (- [ ] Кастомные алиасы &#40;выбор short_code пользователем&#41;)

[//]: # (- [ ] Аутентификация &#40;JWT&#41; и user-scoped ссылки)

[//]: # (- [ ] Rate limiting на `/create`)

[//]: # (- [ ] Кэш горячих ссылок в Redis)

## Лицензия
[MIT LICENSE](LICENSE)