package registry

// Viewer — тот, кто смотрит витрину. Registry не знает, откуда взялась
// личность (Clerk, Keycloak, заглушка) — ему нужно только это.
type Viewer struct {
	Authenticated bool
	Roles         []string
}

// Anonymous — неавторизованный посетитель.
var Anonymous = Viewer{}

// HasAnyRole сообщает, есть ли у зрителя хотя бы одна из ролей.
func (v Viewer) HasAnyRole(roles []string) bool {
	for _, want := range roles {
		for _, have := range v.Roles {
			if want == have {
				return true
			}
		}
	}
	return false
}

// VisibleTo решает, показывать ли элемент зрителю на витрине.
func (it Item) VisibleTo(v Viewer) bool {
	if !it.Status.ShownInCatalog() {
		return false
	}
	// Карточки с ещё не реализованным типом назначения фронт открыть не сможет.
	if !it.TargetType.Supported() {
		return false
	}
	switch it.Visibility {
	case VisibilityPublic:
		return true
	case VisibilityAuthenticated:
		return v.Authenticated
	case VisibilityRoleBased:
		return v.Authenticated && v.HasAnyRole(it.AllowedRoles)
	default:
		return false
	}
}
