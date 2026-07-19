package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/saponite/url-shortner/internal/handler"
	"github.com/saponite/url-shortner/migrations"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("файл .env не был найден: %v\n", err.Error())
	}

	connectString := loadENV()

	pool, err := pgxpool.New(context.Background(), connectString)
	if err != nil {
		log.Fatalf("ошибка подключения к БД: %v", err)
	}
	defer pool.Close()

	if err := runMigrations(connectString); err != nil {
		log.Fatalf("ошибка применения миграций: %v", err)
	}
	log.Println("миграции базы данных успешно проверены/применены")

	router := chi.NewRouter()

	storage := handler.NewPGStorage(pool)
	h := handler.New(storage, os.Getenv("LINK_PREFIX"))
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/index.html")
	})

	router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web"))))
	router.Post("/create", h.CreateShortenedLink)
	router.Get("/{code}", h.GetOriginalURL)
	router.Post("/create_account", h.CreateNewUserAccount)

	fmt.Println("сервер запущен на :1234")
	if err := http.ListenAndServe(":1234", router); err != nil {
		log.Fatal("сервер не был запущен " + err.Error())
	}
}

func runMigrations(connectString string) error {
	db, err := sql.Open("pgx", connectString)
	if err != nil {
		return fmt.Errorf("не удалось открыть соединение для миграций: %w", err)
	}
	defer func() {
		errClose := db.Close()
		if errClose != nil {
			log.Fatal("db не закрыла соединение:", errClose)
		}
	}()

	goose.SetBaseFS(migrations.FS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("не удалось установить диалект goose: %w", err)
	}

	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("не удалось применить миграции: %w", err)
	}

	return nil
}

func loadENV() string {
	return fmt.Sprintf(
		"%s://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("DB"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)
}
