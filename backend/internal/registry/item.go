// Package registry — реестр элементов витрины.
//
// Registry отвечает только за метаданные карточки: что показать, как назвать,
// куда вести, кому показывать, в каком она состоянии и кто за неё отвечает.
// Никакой бизнес-логики конкретных сервисов здесь быть не должно.
package registry

import "time"

// Status — жизненный цикл элемента.
type Status string

const (
	StatusDraft       Status = "draft"       // готовится, пользователям не виден
	StatusActive      Status = "active"      // работает и виден
	StatusMaintenance Status = "maintenance" // виден, но фронт показывает «на обслуживании»
	StatusDisabled    Status = "disabled"    // выключен, пользователям не виден
)

// AllStatuses перечисляет допустимые статусы.
var AllStatuses = []Status{StatusDraft, StatusActive, StatusMaintenance, StatusDisabled}

// Valid сообщает, является ли статус допустимым.
func (s Status) Valid() bool {
	for _, v := range AllStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// ShownInCatalog — попадает ли элемент с таким статусом на витрину.
func (s Status) ShownInCatalog() bool {
	return s == StatusActive || s == StatusMaintenance
}

// Visibility — кому показывать элемент.
type Visibility string

const (
	VisibilityPublic        Visibility = "public"        // всем, включая анонимов
	VisibilityAuthenticated Visibility = "authenticated" // только вошедшим
	VisibilityRoleBased     Visibility = "role_based"    // только вошедшим с одной из AllowedRoles
)

// AllVisibilities перечисляет допустимые значения видимости.
var AllVisibilities = []Visibility{VisibilityPublic, VisibilityAuthenticated, VisibilityRoleBased}

// Valid сообщает, является ли значение видимости допустимым.
func (v Visibility) Valid() bool {
	for _, x := range AllVisibilities {
		if v == x {
			return true
		}
	}
	return false
}

// Owner — ответственный за элемент (команда или человек).
// Нужен для управления, пользователю витрины не отдаётся.
type Owner struct {
	Name    string // команда или человек, например "Профком ФФ" или "Петров Е.А."
	Contact string // необязательно: e-mail, telegram и т.п.
}

// Item — элемент реестра (карточка витрины).
type Item struct {
	ID          string
	Slug        string // стабильный человекочитаемый ключ (используется при сидировании)
	Name        string
	Description string
	Icon        string // emoji или URL картинки

	TargetType TargetType
	Target     string

	Status       Status
	Visibility   Visibility
	AllowedRoles []string // заполняется только при VisibilityRoleBased

	Owner Owner

	SortOrder int
	Pinned    bool

	CreatedAt time.Time
	UpdatedAt time.Time
}
