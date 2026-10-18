// Package config — настройки из переменных окружения.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr           string   // HTTP_ADDR (по умолчанию :8081; если задан PORT — :$PORT)
	Storage            string   // STORAGE: postgres | memory
	DatabaseURL        string   // DATABASE_URL
	MigrateOnStart     bool     // MIGRATE_ON_START (по умолчанию true)
	SeedFile           string   // SEED_FILE — для STORAGE=memory загружается при старте
	AdminToken         string   // ADMIN_TOKEN — Bearer-токен для CRUD; пусто = запись выключена
	AuthDevHeaders     bool     // AUTH_DEV_HEADERS — доверять X-Dev-User/X-Dev-Roles (только dev!)
	CORSAllowedOrigins []string // CORS_ALLOWED_ORIGINS — через запятую, * = любые
	LogLevel           string   // LOG_LEVEL: debug | info | warn | error
	LogFormat          string   // LOG_FORMAT: text | json
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:    env("HTTP_ADDR", ""),
		Storage:     env("STORAGE", "postgres"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		SeedFile:    env("SEED_FILE", "seed/services.json"),
		AdminToken:  os.Getenv("ADMIN_TOKEN"),
		LogLevel:    env("LOG_LEVEL", "info"),
		LogFormat:   env("LOG_FORMAT", "text"),
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = ":" + env("PORT", "8081")
	}
	var err error
	if c.MigrateOnStart, err = envBool("MIGRATE_ON_START", true); err != nil {
		return c, err
	}
	if c.AuthDevHeaders, err = envBool("AUTH_DEV_HEADERS", false); err != nil {
		return c, err
	}
	for _, o := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.CORSAllowedOrigins = append(c.CORSAllowedOrigins, o)
		}
	}

	switch c.Storage {
	case "postgres":
		if c.DatabaseURL == "" {
			return c, fmt.Errorf("DATABASE_URL обязателен при STORAGE=postgres")
		}
	case "memory":
	default:
		return c, fmt.Errorf("STORAGE: допустимые значения postgres, memory (получено %q)", c.Storage)
	}
	if c.AdminToken != "" && len(c.AdminToken) < 16 {
		return c, fmt.Errorf("ADMIN_TOKEN должен быть не короче 16 символов")
	}
	return c, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: ожидается true/false, получено %q", key, v)
	}
	return b, nil
}
