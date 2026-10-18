package registry

import (
	"errors"
	"net/url"
	"strings"
)

// TargetType — тип назначения карточки. Фронтенд решает, как открыть карточку,
// по TargetType, а не по знанию конкретных сервисов.
type TargetType string

const (
	TargetExternalURL   TargetType = "external_url"   // внешняя ссылка: https://example.msu.ru
	TargetInternalRoute TargetType = "internal_route" // маршрут внутри портала: /profile (пока не поддержан)
	TargetApplication   TargetType = "application"    // встроенное приложение портала (пока не поддержан)
	TargetEmbedded      TargetType = "embedded"       // встраивание (iframe/виджет) (пока не поддержан)
)

// KnownTargetTypes — все типы, предусмотренные моделью (и схемой БД).
var KnownTargetTypes = []TargetType{TargetExternalURL, TargetInternalRoute, TargetApplication, TargetEmbedded}

// Known сообщает, предусмотрен ли тип моделью.
func (t TargetType) Known() bool {
	for _, k := range KnownTargetTypes {
		if t == k {
			return true
		}
	}
	return false
}

// TargetHandler проверяет и нормализует target для конкретного типа.
// Чтобы поддержать новый тип назначения, достаточно реализовать этот интерфейс
// и зарегистрировать его в targetHandlers.
type TargetHandler interface {
	Normalize(target string) (string, error)
}

// targetHandlers — реализованные типы назначения.
var targetHandlers = map[TargetType]TargetHandler{
	TargetExternalURL: externalURLHandler{},
}

// Supported сообщает, реализована ли поддержка типа.
func (t TargetType) Supported() bool {
	_, ok := targetHandlers[t]
	return ok
}

// SupportedTargetTypes возвращает реализованные типы в стабильном порядке.
func SupportedTargetTypes() []TargetType {
	var out []TargetType
	for _, t := range KnownTargetTypes {
		if t.Supported() {
			out = append(out, t)
		}
	}
	return out
}

const maxTargetLen = 2048

type externalURLHandler struct{}

func (externalURLHandler) Normalize(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("обязательное поле")
	}
	if len(target) > maxTargetLen {
		return "", errors.New("слишком длинный URL")
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", errors.New("некорректный URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("URL должен начинаться с http:// или https://")
	}
	if u.Host == "" {
		return "", errors.New("в URL не указан хост")
	}
	if u.User != nil {
		return "", errors.New("URL не должен содержать логин/пароль")
	}
	return u.String(), nil
}
