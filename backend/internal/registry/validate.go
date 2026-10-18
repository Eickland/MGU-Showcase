package registry

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// FieldError — ошибка в конкретном поле.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError — набор ошибок валидации.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = f.Field + ": " + f.Message
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

func (e *ValidationError) add(field, msg string) { e.Fields = append(e.Fields, FieldError{field, msg}) }

var (
	slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	roleRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,63}$`)
)

const (
	maxSlugLen        = 64
	maxNameLen        = 120
	maxDescriptionLen = 1000
	maxIconLen        = 512
	maxOwnerLen       = 200
	maxRoles          = 50
)

// normalizeAndValidate приводит элемент к каноническому виду и проверяет его.
// Возвращает *ValidationError, если есть ошибки.
func normalizeAndValidate(it *Item) error {
	ve := &ValidationError{}

	it.Slug = strings.TrimSpace(it.Slug)
	switch {
	case it.Slug == "":
		ve.add("slug", "обязательное поле")
	case len(it.Slug) > maxSlugLen || !slugRe.MatchString(it.Slug):
		ve.add("slug", "только латиница в нижнем регистре, цифры и дефисы, до 64 символов")
	}

	it.Name = strings.TrimSpace(it.Name)
	if it.Name == "" {
		ve.add("name", "обязательное поле")
	} else if utf8.RuneCountInString(it.Name) > maxNameLen {
		ve.add("name", fmt.Sprintf("не длиннее %d символов", maxNameLen))
	}

	it.Description = strings.TrimSpace(it.Description)
	if utf8.RuneCountInString(it.Description) > maxDescriptionLen {
		ve.add("description", fmt.Sprintf("не длиннее %d символов", maxDescriptionLen))
	}

	it.Icon = strings.TrimSpace(it.Icon)
	if len(it.Icon) > maxIconLen {
		ve.add("icon", fmt.Sprintf("не длиннее %d байт", maxIconLen))
	}

	switch {
	case it.TargetType == "":
		ve.add("target_type", "обязательное поле")
	case !it.TargetType.Known():
		ve.add("target_type", "неизвестный тип назначения")
	case !it.TargetType.Supported():
		ve.add("target_type", fmt.Sprintf("тип %q пока не поддерживается", it.TargetType))
	default:
		norm, err := targetHandlers[it.TargetType].Normalize(it.Target)
		if err != nil {
			ve.add("target", err.Error())
		} else {
			it.Target = norm
		}
	}

	if !it.Status.Valid() {
		ve.add("status", "допустимые значения: draft, active, maintenance, disabled")
	}

	if !it.Visibility.Valid() {
		ve.add("visibility", "допустимые значения: public, authenticated, role_based")
	}

	it.AllowedRoles = normalizeRoles(it.AllowedRoles)
	if it.Visibility == VisibilityRoleBased && len(it.AllowedRoles) == 0 {
		ve.add("allowed_roles", "для role_based нужен хотя бы один role")
	}
	if it.Visibility != VisibilityRoleBased && len(it.AllowedRoles) > 0 {
		ve.add("allowed_roles", "роли задаются только при visibility=role_based")
	}
	if len(it.AllowedRoles) > maxRoles {
		ve.add("allowed_roles", fmt.Sprintf("не больше %d ролей", maxRoles))
	}
	for _, r := range it.AllowedRoles {
		if !roleRe.MatchString(r) {
			ve.add("allowed_roles", fmt.Sprintf("некорректное имя роли %q", r))
		}
	}

	it.Owner.Name = strings.TrimSpace(it.Owner.Name)
	it.Owner.Contact = strings.TrimSpace(it.Owner.Contact)
	if it.Owner.Name == "" {
		ve.add("owner.name", "обязательное поле: у каждого элемента должен быть ответственный")
	} else if utf8.RuneCountInString(it.Owner.Name) > maxOwnerLen {
		ve.add("owner.name", fmt.Sprintf("не длиннее %d символов", maxOwnerLen))
	}
	if utf8.RuneCountInString(it.Owner.Contact) > maxOwnerLen {
		ve.add("owner.contact", fmt.Sprintf("не длиннее %d символов", maxOwnerLen))
	}

	if len(ve.Fields) > 0 {
		return ve
	}
	return nil
}

// normalizeRoles обрезает пробелы, убирает пустые и дубликаты, сортирует.
func normalizeRoles(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, r := range in {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}
