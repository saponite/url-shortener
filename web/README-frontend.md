# Фронтенд

Статический фронт: `index.html` + `app.js` (ES-модуль) + `styles.css` (собран Tailwind).
Без `node_modules` и бандлеров — CSS собирается standalone-бинарём Tailwind.

## Файлы

| Файл | Назначение |
|------|-----------|
| `index.html` | Разметка. Ассеты подключены как `/static/styles.css`, `/static/app.js`. |
| `app.js` | Логика: api / session / валидация / состояния / обработчики. |
| `src/input.css` | **Источник** стилей: директивы Tailwind + дизайн-токены. Правим здесь. |
| `styles.css` | **Сгенерированный** CSS. Не редактируем руками — коммитим результат сборки. |
| `tailwind.config.js` | Конфиг Tailwind (content, darkMode: class, токены). |

## Сборка CSS

Нужен standalone-бинарь Tailwind (без Node). Скачать один раз:

```bash
# macOS arm64 (пример; выбери свою платформу из релизов tailwindlabs/tailwindcss)
curl -sL -o tailwindcss \
  https://github.com/tailwindlabs/tailwindcss/releases/download/v3.4.17/tailwindcss-macos-arm64
chmod +x tailwindcss
```

Собрать (из корня репозитория):

```bash
./tailwindcss -i web/src/input.css -o web/styles.css --minify
```

Во время разработки удобно с `--watch`:

```bash
./tailwindcss -i web/src/input.css -o web/styles.css --watch
```

> Если Node всё же есть и standalone-бинарь не нужен: `npx tailwindcss@3 -i web/src/input.css -o web/styles.css --minify`.
>
> **Фолбэк без сборки:** заменить в `index.html` `<link rel="stylesheet" href="/static/styles.css">`
> на `<script src="https://cdn.tailwindcss.com"></script>` — но токены из `src/input.css` тогда
> не подхватятся, так что это только для быстрой локальной проверки, не для прода.

## Отдача статики (backend, `cmd/url-shortener/main.go`)

Сейчас сервер отдаёт только `web/index.html`. Нужно добавить отдачу статики под префиксом
`/static/*` — **важно именно под префиксом**, потому что маршрут `GET /{code}` в chi перехватывает
любой одиночный сегмент (`/styles.css`, `/app.js`) и без префикса ассеты не загрузятся.

Рекомендуемый вариант — `embed.FS` (заодно чинит хрупкий относительный путь `"web/index.html"`,
который ломается при запуске не из корня, например в Docker):

```go
import (
    "embed"
    "io/fs"
    "net/http"
)

//go:embed web
var webFS embed.FS

func main() {
    // ...router := chi.NewRouter()...

    staticFS, err := fs.Sub(webFS, "web")
    if err != nil {
        log.Fatal(err)
    }
    router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

    router.Get("/", func(w http.ResponseWriter, r *http.Request) {
        b, err := webFS.ReadFile("web/index.html")
        if err != nil {
            http.Error(w, "not found", http.StatusNotFound)
            return
        }
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write(b)
    })

    // router.Post("/create", ...) — без изменений
    // router.Get("/{code}", ...) — без изменений
}
```

Проще (без embed, читает с диска):

```go
router.Handle("/static/*", http.StripPrefix("/static/",
    http.FileServer(http.Dir("web"))))
```

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
2. вызвать `session.refresh()` (добавить) в `init()` вместо `renderAccount()` напрямую.

Запросы уже шлются с `credentials: "same-origin"`, так что cookie поедут автоматически.
