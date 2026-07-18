package handler

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

var (
	domainRegex = regexp.MustCompile(
		`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`,
	)
	schemeRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*://`)
)

type Link struct {
	OriginalURL string    `json:"original_url"`
	ShortCode   string    `json:"short_code"`
	CreatedAt   time.Time `json:"created_at"`
}

type LinkResponse struct {
	OriginalURL string    `json:"original_url"`
	ShortURL    string    `json:"short_url"`
	CreatedAt   time.Time `json:"created_at"`
}

type Handler struct {
	linkStorage LinkStorage
	userStorage UserStorage
	linkPrefix  string
}

type User struct {
	LastName  string `json:"last_name"`
	FirstName string `json:"first_name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

func New(storage LinkStorage, linkPrefix string) *Handler {
	return &Handler{linkStorage: storage, linkPrefix: linkPrefix}
}

func (h *Handler) CreateShortenedLink(w http.ResponseWriter, r *http.Request) {
	var link Link
	if err := json.NewDecoder(r.Body).Decode(&link); err != nil {
		http.Error(w, "недействительный JSON", http.StatusBadRequest)
		return
	}

	link.OriginalURL = strings.TrimSpace(link.OriginalURL)
	if link.OriginalURL == "" {
		http.Error(w, "original_url пуст", http.StatusBadRequest)
		return
	}

	body := stripPrefix(link.OriginalURL)
	if body == "" {
		http.Error(w, "недействительный URL", http.StatusBadRequest)
		return
	}

	host := body
	if i := strings.IndexAny(host, "/?#"); i != -1 {
		host = host[:i]
	}
	if !isValidHost(host) {
		http.Error(w, "недействительный домен", http.StatusBadRequest)
		return
	}

	link.OriginalURL = strings.TrimRight(link.OriginalURL, "/")
	body = strings.TrimRight(body, "/")

	ctx := r.Context()

	exists, err := h.linkStorage.GetByOriginalURL(ctx, link.OriginalURL)
	if err != nil {
		log.Printf("ошибка БД: %v", err)
		http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
		return
	}
	if exists != nil {
		writeJSON(w, http.StatusOK, h.toResponse(exists))
		return
	}

	code, err := h.createWithRetry(ctx, link.OriginalURL, body, 5)
	if err != nil {
		log.Printf("ошибка БД: %v", err)
		http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
		return
	}

	created, err := h.linkStorage.GetByCode(ctx, code)
	if err != nil {
		log.Printf("ошибка БД: %v", err)
		http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
		return
	}
	if created == nil {
		log.Printf("сразу после вставки запись не найдена: %s", code)
		http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, h.toResponse(created))
}

func (h *Handler) GetOriginalURL(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()
	link, err := h.linkStorage.GetByCode(ctx, code)
	if err != nil {
		log.Printf("ошибка БД: %v", err)
		http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
		return
	}
	if link == nil {
		http.NotFound(w, r)
		return
	}

	target := link.OriginalURL
	if !schemeRegex.MatchString(target) {
		target = "https://" + target
	}

	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handler) CreateNewUserAccount(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "недействительный JSON", http.StatusBadRequest)
		return
	}

	domains := map[string]struct{}{
		"yandex.ru":   {},
		"mail.ru":     {},
		"gmail.com":   {},
		"internet.ru": {},
		"bk.ru":       {},
		"list.ru":     {},
		"inbox.ru":    {},
		"icloud.com":  {},
	}

	pos := strings.Index(user.Email, "@")
	domain := user.Email[pos+1:]
	if _, found := domains[domain]; !found {
		http.Error(w, "такой домен эл.почты не поддерживается", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	err := h.userStorage.CreateNewUserAccount(ctx, user.FirstName, user.LastName, user.Email, user.Password)
	if err != nil {
		log.Printf("ошибка БД: %v", err)
		if errors.Is(err, ErrUserAlreadyExists) {
			http.Error(w, "пользователь с таким email существует", http.StatusBadRequest)
			return
		}
		http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, "пользователь зарегистрирован")
}

func (h *Handler) createWithRetry(ctx context.Context, originalURL, hashInput string, maxRetries int) (string, error) {
	for i := 0; i < maxRetries; i++ {
		code := generateShortCode([]byte(hashInput), i)

		inserted, err := h.linkStorage.CreateNewShortLink(ctx, code, originalURL)
		if err == nil {
			return inserted, nil
		}
		if errors.Is(err, ErrCodeTaken) {
			continue
		}
		return "", err
	}
	return "", errors.New("не удалось сгенерировать короткий код")
}

func stripPrefix(raw string) string {
	s := raw
	if loc := schemeRegex.FindStringIndex(s); loc != nil {
		s = s[loc[1]:]
	}
	if len(s) >= 4 && strings.EqualFold(s[:4], "www.") {
		s = s[4:]
	}
	return s
}

func isValidHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return domainRegex.MatchString(host)
}

func generateShortCode(url []byte, salt int) string {
	payload := append([]byte(nil), url...)
	payload = binary.BigEndian.AppendUint32(payload, uint32(salt))

	hash := sha256.Sum256(payload)

	const length = 7
	b := make([]byte, length)
	for i := 0; i < length; i++ {
		b[i] = alphabet[hash[i]%byte(len(alphabet))]
	}
	return string(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("ошибка при кодировке ответа: %v", err)
	}
}

func (h *Handler) toResponse(l *Link) LinkResponse {
	base, _ := url.Parse(h.linkPrefix)
	base.Path = strings.TrimRight(base.Path, "/") + "/" + l.ShortCode

	return LinkResponse{
		OriginalURL: l.OriginalURL,
		ShortURL:    base.String(),
		CreatedAt:   l.CreatedAt,
	}
}
