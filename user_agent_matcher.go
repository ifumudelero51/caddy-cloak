package botredirect

import (
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// UserAgentMatcher отвечает за анализ User-Agent строк для определения ботов
type UserAgentMatcher struct {
	// Паттерны для поиска ботов
	botPatterns     []string
	compiledRegexps []*regexp.Regexp

	// Exact matches для быстрой проверки
	exactMatches map[string]bool

	// Contains matches для подстрок
	containsMatches []string

	// Синхронизация
	mutex sync.RWMutex

	// Кеш результатов
	cache    map[string]*UserAgentResult
	cacheTTL time.Duration
	maxCache int

	// Компоненты
	metrics *Metrics
	debug   *DebugConfig
	logger  *zap.Logger

	// Статистика (используем atomic для thread-safety)
	totalChecks   int64
	botDetections int64
	cacheHits     int64
}

// UserAgentResult содержит результат анализа User-Agent
type UserAgentResult struct {
	IsBot          bool
	MatchedPattern string
	BotType        BotType
	Confidence     float64
	Timestamp      time.Time
}

// NewUserAgentMatcher создает новый экземпляр UserAgentMatcher
func NewUserAgentMatcher(config *Config, metrics *Metrics, debug *DebugConfig, logger *zap.Logger) *UserAgentMatcher {
	uam := &UserAgentMatcher{
		botPatterns:     make([]string, 0),
		compiledRegexps: make([]*regexp.Regexp, 0),
		exactMatches:    make(map[string]bool),
		containsMatches: make([]string, 0),
		cache:           make(map[string]*UserAgentResult),
		cacheTTL:        config.CacheTTL,
		maxCache:        1000, // Максимум 1000 записей в кеше
		metrics:         metrics,
		debug:           debug,
		logger:          logger,
	}

	// Используем кастомные паттерны если заданы, иначе дефолтные
	patterns := config.BotUserAgents
	if len(patterns) == 0 {
		patterns = getDefaultBotUserAgents()
	}

	// Инициализация паттернов
	if err := uam.initializePatterns(patterns); err != nil {
		logger.Error("failed to initialize user agent patterns", zap.Error(err))
		return uam
	}

	logger.Info("user agent matcher initialized",
		zap.Int("total_patterns", len(uam.botPatterns)),
		zap.Int("regex_patterns", len(uam.compiledRegexps)),
		zap.Int("exact_matches", len(uam.exactMatches)),
		zap.Int("contains_matches", len(uam.containsMatches)),
	)

	return uam
}

// initializePatterns инициализирует и оптимизирует паттерны
func (uam *UserAgentMatcher) initializePatterns(patterns []string) error {
	uam.mutex.Lock()
	defer uam.mutex.Unlock()

	// Очистка предыдущих паттернов
	uam.botPatterns = make([]string, 0, len(patterns))
	uam.compiledRegexps = make([]*regexp.Regexp, 0)
	uam.exactMatches = make(map[string]bool)
	uam.containsMatches = make([]string, 0)

	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}

		uam.botPatterns = append(uam.botPatterns, pattern)

		// Оптимизация: разные типы паттернов для разной скорости проверки
		if uam.isExactMatch(pattern) {
			// Точное совпадение - самый быстрый
			uam.exactMatches[strings.ToLower(pattern)] = true
		} else if uam.isSimpleContains(pattern) {
			// Простое вхождение - быстрый
			cleanPattern := strings.Trim(pattern, "*")
			uam.containsMatches = append(uam.containsMatches, strings.ToLower(cleanPattern))
		} else {
			// Регулярное выражение - медленный но гибкий
			if regex, err := regexp.Compile("(?i)" + pattern); err == nil {
				uam.compiledRegexps = append(uam.compiledRegexps, regex)
			} else {
				uam.logger.Warn("invalid regex pattern",
					zap.String("pattern", pattern),
					zap.Error(err),
				)
			}
		}
	}

	return nil
}

// isExactMatch проверяет, является ли паттерн точным совпадением
func (uam *UserAgentMatcher) isExactMatch(pattern string) bool {
	// Паттерн считается точным, если не содержит спецсимволов regex
	specialChars := []string{"*", "?", "+", "[", "]", "(", ")", "{", "}", "^", "$", "|", "\\", "."}
	for _, char := range specialChars {
		if strings.Contains(pattern, char) {
			return false
		}
	}
	return true
}

// isSimpleContains проверяет, является ли паттерн простым вхождением
func (uam *UserAgentMatcher) isSimpleContains(pattern string) bool {
	// Простые паттерны содержат только * в начале или конце
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		core := pattern[1 : len(pattern)-1]
		return !strings.Contains(core, "*") && !strings.ContainsAny(core, "?+[](){}^$|\\.")
	}
	if strings.HasPrefix(pattern, "*") || strings.HasSuffix(pattern, "*") {
		core := strings.Trim(pattern, "*")
		return !strings.ContainsAny(core, "?+[](){}^$|\\.*")
	}
	return false
}

// IsBot проверяет, является ли User-Agent ботом
func (uam *UserAgentMatcher) IsBot(userAgent string) (*UserAgentResult, error) {
	if userAgent == "" {
		return &UserAgentResult{
			IsBot:      false,
			BotType:    BotTypeUnknown,
			Confidence: 0.0,
			Timestamp:  time.Now(),
		}, nil
	}

	// Инкремент счетчика проверок
	atomic.AddInt64(&uam.totalChecks, 1)

	// Инкремент метрик для детального анализа
	if uam.metrics != nil {
		uam.metrics.IncrementUserAgentChecks()
	}

	// Проверка кеша
	if result := uam.getCachedResult(userAgent); result != nil {
		atomic.AddInt64(&uam.cacheHits, 1)
		if uam.metrics != nil {
			uam.metrics.IncrementCacheHits()
		}

		if uam.debug != nil {
			uam.debug.LogUserAgentCheck(userAgent, result.IsBot, result.MatchedPattern)
		}

		return result, nil
	}

	if uam.metrics != nil {
		uam.metrics.IncrementCacheMisses()
	}

	// Выполнение проверки
	result := uam.performCheck(userAgent)

	// Сохранение в кеш
	uam.setCachedResult(userAgent, result)

	// Логирование для дебага
	if uam.debug != nil {
		uam.debug.LogUserAgentCheck(userAgent, result.IsBot, result.MatchedPattern)
	}

	// Обновление статистики
	if result.IsBot {
		atomic.AddInt64(&uam.botDetections, 1)
	}

	return result, nil
}

// performCheck выполняет основную проверку User-Agent
func (uam *UserAgentMatcher) performCheck(userAgent string) *UserAgentResult {
	uam.mutex.RLock()
	defer uam.mutex.RUnlock()

	userAgentLower := strings.ToLower(userAgent)

	// 1. Проверка точных совпадений (самый быстрый)
	if uam.exactMatches[userAgentLower] {
		return &UserAgentResult{
			IsBot:          true,
			MatchedPattern: userAgentLower,
			BotType:        uam.determineBotType(userAgent),
			Confidence:     1.0,
			Timestamp:      time.Now(),
		}
	}

	// 2. Проверка простых вхождений
	for _, pattern := range uam.containsMatches {
		if strings.Contains(userAgentLower, pattern) {
			return &UserAgentResult{
				IsBot:          true,
				MatchedPattern: pattern,
				BotType:        uam.determineBotType(userAgent),
				Confidence:     0.9,
				Timestamp:      time.Now(),
			}
		}
	}

	// 3. Проверка регулярных выражений (самый медленный)
	for _, regex := range uam.compiledRegexps {
		if regex.MatchString(userAgent) {
			return &UserAgentResult{
				IsBot:          true,
				MatchedPattern: regex.String(),
				BotType:        uam.determineBotType(userAgent),
				Confidence:     0.8,
				Timestamp:      time.Now(),
			}
		}
	}

	// Не найдено совпадений
	return &UserAgentResult{
		IsBot:      false,
		BotType:    BotTypeUnknown,
		Confidence: 0.0,
		Timestamp:  time.Now(),
	}
}

// determineBotType определяет тип бота на основе User-Agent
func (uam *UserAgentMatcher) determineBotType(userAgent string) BotType {
	userAgentLower := strings.ToLower(userAgent)

	// Поисковые боты
	searchBots := []string{
		"googlebot", "bingbot", "yandexbot", "duckduckbot", "baiduspider",
		"sogou", "360spider", "slurp", "crawler", "spider",
	}

	for _, bot := range searchBots {
		if strings.Contains(userAgentLower, bot) {
			return BotTypeSearch
		}
	}

	// Социальные боты
	socialBots := []string{
		"facebookexternalhit", "twitterbot", "linkedinbot", "whatsapp",
		"telegrambot", "vkshare", "applebot", "skypeuripreview",
	}

	for _, bot := range socialBots {
		if strings.Contains(userAgentLower, bot) {
			return BotTypeSocial
		}
	}

	// SEO боты
	seoBots := []string{
		"ahrefs", "semrush", "moz", "majestic", "screaming",
	}

	for _, bot := range seoBots {
		if strings.Contains(userAgentLower, bot) {
			return BotTypeSEO
		}
	}

	// Мониторинг боты
	monitoringBots := []string{
		"pingdom", "uptimerobot", "monitor", "check", "test",
	}

	for _, bot := range monitoringBots {
		if strings.Contains(userAgentLower, bot) {
			return BotTypeMonitoring
		}
	}

	// Остальные краулеры
	if strings.Contains(userAgentLower, "bot") ||
		strings.Contains(userAgentLower, "crawl") ||
		strings.Contains(userAgentLower, "spider") {
		return BotTypeCrawler
	}

	return BotTypeUnknown
}

// getCachedResult получает результат из кеша
func (uam *UserAgentMatcher) getCachedResult(userAgent string) *UserAgentResult {
	uam.mutex.RLock()
	defer uam.mutex.RUnlock()

	if result, exists := uam.cache[userAgent]; exists {
		// Проверка TTL
		if time.Since(result.Timestamp) < uam.cacheTTL {
			return result
		}
		// Удаление устаревшей записи
		delete(uam.cache, userAgent)
	}

	return nil
}

// setCachedResult сохраняет результат в кеш
func (uam *UserAgentMatcher) setCachedResult(userAgent string, result *UserAgentResult) {
	uam.mutex.Lock()
	defer uam.mutex.Unlock()

	// Проверка размера кеша
	if len(uam.cache) >= uam.maxCache {
		uam.cleanupCacheUnsafe()
	}

	uam.cache[userAgent] = result

	if uam.debug != nil {
		uam.debug.LogCacheOperation(&CacheDebugInfo{
			Key:       userAgent,
			Operation: "set",
			Hit:       false,
			Value:     result,
			TTL:       uam.cacheTTL,
			Timestamp: time.Now(),
		})
	}
}

// cleanupCacheUnsafe очищает старые записи из кеша (вызывать под мьютексом)
func (uam *UserAgentMatcher) cleanupCacheUnsafe() {
	now := time.Now()

	for key, result := range uam.cache {
		if now.Sub(result.Timestamp) > uam.cacheTTL {
			delete(uam.cache, key)
		}
	}

	// Если кеш все еще переполнен, удаляем самые старые записи
	if len(uam.cache) >= uam.maxCache {
		// Простая стратегия: удаляем половину записей
		count := 0
		target := len(uam.cache) / 2

		for key := range uam.cache {
			if count >= target {
				break
			}
			delete(uam.cache, key)
			count++
		}
	}
}

// AddPattern добавляет новый паттерн в runtime
func (uam *UserAgentMatcher) AddPattern(pattern string) error {
	uam.mutex.Lock()
	defer uam.mutex.Unlock()

	if pattern == "" {
		return nil
	}

	// Добавляем в список паттернов
	uam.botPatterns = append(uam.botPatterns, pattern)

	// Классифицируем и добавляем в соответствующую структуру
	if uam.isExactMatch(pattern) {
		uam.exactMatches[strings.ToLower(pattern)] = true
	} else if uam.isSimpleContains(pattern) {
		cleanPattern := strings.Trim(pattern, "*")
		uam.containsMatches = append(uam.containsMatches, strings.ToLower(cleanPattern))
	} else {
		if regex, err := regexp.Compile("(?i)" + pattern); err == nil {
			uam.compiledRegexps = append(uam.compiledRegexps, regex)
		} else {
			return err
		}
	}

	// Очищаем кеш после добавления нового паттерна
	uam.cache = make(map[string]*UserAgentResult)

	uam.logger.Info("added new user agent pattern",
		zap.String("pattern", pattern),
		zap.Int("total_patterns", len(uam.botPatterns)),
	)

	return nil
}

// RemovePattern удаляет паттерн
func (uam *UserAgentMatcher) RemovePattern(pattern string) {
	uam.mutex.Lock()
	defer uam.mutex.Unlock()

	// Удаляем из основного списка
	for i, p := range uam.botPatterns {
		if p == pattern {
			uam.botPatterns = append(uam.botPatterns[:i], uam.botPatterns[i+1:]...)
			break
		}
	}

	// Переинициализируем все структуры
	patterns := make([]string, len(uam.botPatterns))
	copy(patterns, uam.botPatterns)

	// Освобождаем мьютекс временно для вызова initializePatterns
	uam.mutex.Unlock()
	uam.initializePatterns(patterns)
	uam.mutex.Lock()

	uam.logger.Info("removed user agent pattern",
		zap.String("pattern", pattern),
		zap.Int("total_patterns", len(uam.botPatterns)),
	)
}

// GetStats возвращает статистику
func (uam *UserAgentMatcher) GetStats() map[string]interface{} {
	uam.mutex.RLock()
	cacheSize := len(uam.cache)
	totalPatterns := len(uam.botPatterns)
	exactMatches := len(uam.exactMatches)
	containsMatches := len(uam.containsMatches)
	regexPatterns := len(uam.compiledRegexps)
	uam.mutex.RUnlock()

	totalChecks := atomic.LoadInt64(&uam.totalChecks)
	botDetections := atomic.LoadInt64(&uam.botDetections)
	cacheHits := atomic.LoadInt64(&uam.cacheHits)

	hitRate := 0.0
	if totalChecks > 0 {
		hitRate = float64(cacheHits) / float64(totalChecks)
	}

	detectionRate := 0.0
	if totalChecks > 0 {
		detectionRate = float64(botDetections) / float64(totalChecks)
	}

	return map[string]interface{}{
		"total_patterns":   totalPatterns,
		"exact_matches":    exactMatches,
		"contains_matches": containsMatches,
		"regex_patterns":   regexPatterns,
		"cache_size":       cacheSize,
		"cache_max_size":   uam.maxCache,
		"total_checks":     totalChecks,
		"bot_detections":   botDetections,
		"cache_hits":       cacheHits,
		"cache_hit_rate":   hitRate,
		"detection_rate":   detectionRate,
	}
}

// ClearCache очищает кеш
func (uam *UserAgentMatcher) ClearCache() {
	uam.mutex.Lock()
	defer uam.mutex.Unlock()

	uam.cache = make(map[string]*UserAgentResult)
	uam.logger.Info("user agent matcher cache cleared")
}
