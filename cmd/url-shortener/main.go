package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/saponite/url-shortner/internal/handler"
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

	m, err := migrate.New(
		"file://migrations",
		connectString,
	)
	if err != nil {
		log.Fatalf("не удалось инициализировать экземпляр миграций: %v", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("ошибка применения миграций: %v", err)
	}

	sourceError, databaseError := m.Close()
	if sourceError != nil {
		log.Fatal("миграция не закрыта: ", sourceError)
	}
	if databaseError != nil {
		log.Fatal("миграция не закрыта: ", databaseError)
	}

	log.Println("миграции базы данных успешно проверены/применены")

	router := chi.NewRouter()

	storage := handler.NewPGStorage(pool)
	h := handler.New(storage, os.Getenv("LINK_PREFIX"))
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/index.html")
	})
	router.Post("/create", h.CreateShortenedLink)
	router.Get("/{code}", h.GetOriginalURL)

	fmt.Println("сервер запущен на :1234")
	err = http.ListenAndServe(":1234", router)
	if err != nil {
		log.Fatal("сервер не был запущен " + err.Error())
	}
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
