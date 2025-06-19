package botredirect

// BotType представляет тип бота
type BotType string

const (
	BotTypeSearch      BotType = "search"
	BotTypeSocial      BotType = "social"
	BotTypeCrawler     BotType = "crawler"
	BotTypeMonitoring  BotType = "monitoring"
	BotTypeSEO         BotType = "seo"
	BotTypeUnknown     BotType = "unknown"
)

func (bt BotType) String() string {
	return string(bt)
}

// UserType определяет тип пользователя
type UserType int

const (
	UserTypeBot UserType = iota
	UserTypeFromSearch
	UserTypeDirect
)

func (ut UserType) String() string {
	switch ut {
	case UserTypeBot:
		return "bot"
	case UserTypeFromSearch:
		return "from_search"
	case UserTypeDirect:
		return "direct"
	default:
		return "unknown"
	}
}