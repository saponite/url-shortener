# Фронтенд

Статика без сборки и зависимостей: `index.html` + `app.js` (ES-модуль) + `styles.css`
(рукописный CSS, без Tailwind). Ничего компилировать не нужно — правишь файл и деплоишь.

## Файлы

| Файл | Назначение |
|------|-----------|
| `index.html` | Разметка. Ассеты подключены из корня: `/styles.css`, `/app.js`. |
| `app.js` | Логика: api / session / валидация / состояния / обработчики. |
| `styles.css` | Дизайн-система на чистом CSS: токены (light/dark), компоненты, a11y. |
| `src/input.css`, `tailwind.config.js` | ⚠️ Легаси от Tailwind-подхода, **больше не используются** — можно удалить. |

Правишь стиль → меняешь `styles.css`. Меняешь разметку → держи имена классов в согласии с `styles.css`.

## Как это отдаётся (важно!)

В проде перед Go-бэкендом стоит **nginx** (`nginx/Dockerfile`, `nginx/templates/default.conf.template`):

- `COPY web /usr/share/nginx/html/app` — nginx кладёт папку `web/` в свой webroot;
- `root .../app; location / { try_files $uri $uri/ @backend; }` — nginx сам отдаёт файлы из `web/`
  (`/`, `/styles.css`, `/app.js`), а всё остальное (`/create`, `/{code}`, ...) проксирует в Go.

Поэтому ассеты подключаются **из корня** (`/styles.css`, `/app.js`), а не из `/static/` — именно так
они лежат в webroot nginx. Раньше был баг: `index.html` ссылался на `/static/...`, которых в nginx
нет → 404 → страница без стилей и без работающего JS.

Go-бэкенд (`cmd/url-shortener/main.go`) отдаёт те же файлы для прямого доступа на `:1234` (dev):

```go
router.Get("/", func(w, r) { http.ServeFile(w, r, "web/index.html") })
router.Get("/styles.css", func(w, r) { http.ServeFile(w, r, "web/styles.css") })
router.Get("/app.js", func(w, r) { http.ServeFile(w, r, "web/app.js") })
```

Литеральные роуты chi матчатся раньше catch-all `/{code}`, поэтому `/styles.css` не перехватывается
редиректом. При прямом доступе на `:1234` рядом должна быть папка `web/` (она есть при запуске из
репозитория; в Docker-образ бэкенда `web/` не копируется — там статику отдаёт nginx).

## Контракт эндпоинтов, которые ждёт фронт

Работают сейчас:

- `POST /create` — `{ "original_url" }` → `{ "original_url","short_url","created_at" }`.
- `POST /create_account` — `{ "first_name","last_name","email","password" }` → `201`.

Фронт уже написан под них — реализуй на бэке, и они «оживут» без правок фронта:

| Метод | Путь | Тело | Ответ | Заметки |
|-------|------|------|-------|---------|
| POST | `/login` | `{ email, password }` | `200` + `Set-Cookie` (HttpOnly, Secure, SameSite=Lax); `401` при неверных | Сессия в Redis |
| GET | `/me` | — | `200 { email, first_name }` или `401` | Заменит `localStorage` в `session`-модуле (`app.js`) |
| POST | `/logout` | — | `204`; гасит сессию и cookie | |
| GET | `/links` | — | `200 [{ original_url, short_url, short_code, created_at, click_count }]` | Только для авторизованного |

Доступ: **сокращать — всем; история — только авторизованным.** При наличии cookie бэкенд
привязывает создаваемую ссылку к `user_id` (колонка уже есть в `short_links`).

### Как подключить cookie-сессию на фронте

В `app.js` модуль `session` сейчас читает `localStorage`. Когда появятся `/me` и `/logout`:
1. `session.current()` → делать `await api.me()` и хранить результат;
2. вызывать это при старте вместо чтения `localStorage`.

Запросы уже шлются с `credentials: "same-origin"`, так что cookie поедут автоматически.
