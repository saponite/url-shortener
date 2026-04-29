package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
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

	router := chi.NewRouter()

	h := handler.New(pool)
	router.Post("/create", h.HandlerCreateShortenedLink)

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
