package tui

func editorTitle(kind string) string {
	switch kind {
	case "profile":
		return "сервис"
	case "strategy":
		return "стратегия"
	case "config":
		return "все настройки"
	}
	return kind
}

// ThemeTitle localizes theme labels without changing saved IDs.
func ThemeTitle(name string) string {
	switch name {
	case "Auto":
		return "Автоматически"
	case "Monochrome":
		return "Монохромная"
	case "Terminal":
		return "Палитра терминала"
	}
	return name
}
func profileTitle(id string) string {
	switch id {
	case "discord":
		return "Discord"
	case "youtube":
		return "YouTube"
	case "twitch":
		return "Twitch"
	case "telegram":
		return "Telegram"
	case "steam":
		return "Steam"
	case "github":
		return "GitHub"
	case "custom":
		return "Свои сайты"
	}
	return id
}
