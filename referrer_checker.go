package botredirect

import (
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ReferrerChecker отвечает за анализ HTTP Referer заголовков
type ReferrerChecker struct {
	// Конфигурация
	enabled bool
	
	// Разрешенные домены поисковых систем
	allowedDomains    []string
	compiledPatterns  []*regexp.Regexp
	exactDomains      map[string]bool
	wildcardDomains   []string
	
	// Кеш результатов
	cache     map[string]*ReferrerResult
	cacheTTL  time.Duration
	maxCache  int
	
	// Синхронизация
	mutex sync.RWMutex
	
	// Компоненты
	metrics *Metrics
	debug   *DebugConfig
	logger  *zap.Logger
	
	// Статистика
	totalChecks       int64
	validReferrers    int64
	invalidReferrers  int64
	emptyReferrers    int64
	cacheHits         int64
	malformedURLs     int64
	searchEngineHits  map[string]int64
}

// ReferrerResult содержит результат анализа referrer
type ReferrerResult struct {
	IsFromSearch     bool
	SearchEngine     string
	Domain           string
	OriginalURL      string
	MatchedPattern   string
	Confidence       float64
	ReferrerType     ReferrerType
	QueryParameters  map[string]string
	Timestamp        time.Time
}

// ReferrerType представляет тип источника referrer
type ReferrerType string

const (
	ReferrerTypeEmpty        ReferrerType = "empty"
	ReferrerTypeSearchEngine ReferrerType = "search_engine"
	ReferrerTypeSocialMedia  ReferrerType = "social_media"
	ReferrerTypeDirectLink   ReferrerType = "direct_link"
	ReferrerTypeInternal     ReferrerType = "internal"
	ReferrerTypeMalformed    ReferrerType = "malformed"
	ReferrerTypeUnknown      ReferrerType = "unknown"
)

// SearchEngineInfo содержит информацию о поисковой системе
type SearchEngineInfo struct {
	Name            string
	Domains         []string
	QueryParams     []string
	EngineType      string
	Country         string
	MarketShare     float64
}

// NewReferrerChecker создает новый экземпляр ReferrerChecker
func NewReferrerChecker(config *Config, metrics *Metrics, debug *DebugConfig, logger *zap.Logger) *ReferrerChecker {
	if !config.EnableReferrerCheck {
		return &ReferrerChecker{enabled: false}
	}

	rc := &ReferrerChecker{
		enabled:          true,
		allowedDomains:   make([]string, 0),
		compiledPatterns: make([]*regexp.Regexp, 0),
		exactDomains:     make(map[string]bool),
		wildcardDomains:  make([]string, 0),
		cache:            make(map[string]*ReferrerResult),
		cacheTTL:         config.CacheTTL,
		maxCache:         3000, // Кеш для 3000 referrer'ов
		metrics:          metrics,
		debug:            debug,
		logger:           logger,
		searchEngineHits: make(map[string]int64),
	}

	// Используем кастомные домены если заданы, иначе дефолтные
	domains := config.AllowedReferrers
	if len(domains) == 0 {
		domains = getDefaultAllowedReferrers()
	}

	// Инициализация паттернов
	if err := rc.initializePatterns(domains); err != nil {
		logger.Error("failed to initialize referrer patterns", zap.Error(err))
		return rc
	}

	logger.Info("referrer checker initialized",
		zap.Bool("enabled", true),
		zap.Int("total_domains", len(rc.allowedDomains)),
		zap.Int("exact_domains", len(rc.exactDomains)),
		zap.Int("wildcard_domains", len(rc.wildcardDomains)),
		zap.Int("regex_patterns", len(rc.compiledPatterns)),
	)

	return rc
}

// initializePatterns инициализирует и оптимизирует паттерны доменов
func (rc *ReferrerChecker) initializePatterns(domains []string) error {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	// Очистка предыдущих паттернов
	rc.allowedDomains = make([]string, 0, len(domains))
	rc.compiledPatterns = make([]*regexp.Regexp, 0)
	rc.exactDomains = make(map[string]bool)
	rc.wildcardDomains = make([]string, 0)

	for _, domain := range domains {
		if domain == "" {
			continue
		}

		rc.allowedDomains = append(rc.allowedDomains, domain)

		// Классификация паттернов для оптимизации
		if rc.isExactDomain(domain) {
			// Точный домен - самый быстрый поиск
			rc.exactDomains[strings.ToLower(domain)] = true
		} else if rc.isWildcardDomain(domain) {
			// Wildcard домен - быстрый поиск
			rc.wildcardDomains = append(rc.wildcardDomains, strings.ToLower(domain))
		} else {
			// Regex паттерн - медленный но гибкий
			pattern := rc.convertToRegex(domain)
			if regex, err := regexp.Compile("(?i)" + pattern); err == nil {
				rc.compiledPatterns = append(rc.compiledPatterns, regex)
			} else {
				rc.logger.Warn("invalid referrer pattern",
					zap.String("domain", domain),
					zap.String("pattern", pattern),
					zap.Error(err),
				)
			}
		}
	}

	return nil
}

// isExactDomain проверяет, является ли паттерн точным доменом
func (rc *ReferrerChecker) isExactDomain(domain string) bool {
	// Не содержит wildcard символов
	return !strings.Contains(domain, "*") && !strings.Contains(domain, "?") &&
		   !strings.Contains(domain, "[") && !strings.Contains(domain, "(")
}

// isWildcardDomain проверяет, является ли паттерн простым wildcard
func (rc *ReferrerChecker) isWildcardDomain(domain string) bool {
	// Содержит только * символы
	return strings.Contains(domain, "*") && 
		   !strings.Contains(domain, "?") &&
		   !strings.Contains(domain, "[") && 
		   !strings.Contains(domain, "(")
}

// convertToRegex преобразует wildcard паттерн в regex
func (rc *ReferrerChecker) convertToRegex(domain string) string {
	// Экранируем специальные символы regex
	escaped := regexp.QuoteMeta(domain)
	// Заменяем экранированные * на .*
	regex := strings.ReplaceAll(escaped, `\*`, `.*`)
	// Добавляем начало и конец строки
	return "^" + regex + "$"
}

// CheckReferrer проверяет referrer заголовок
func (rc *ReferrerChecker) CheckReferrer(referrer string) (*ReferrerResult, error) {
	if !rc.enabled {
		return &ReferrerResult{IsFromSearch: true}, nil // По умолчанию разрешаем если отключено
	}

	// Инкремент счетчика проверок
	rc.incrementTotalChecks()

	// Инкремент метрик для детального анализа
	if rc.metrics != nil {
		rc.metrics.IncrementReferrerChecks()
	}

	// Обработка пустого referrer
	if referrer == "" {
		rc.incrementEmptyReferrers()
		result := &ReferrerResult{
			IsFromSearch:    false,
			ReferrerType:    ReferrerTypeEmpty,
			OriginalURL:     "",
			Confidence:      1.0,
			Timestamp:       time.Now(),
		}
		
		if rc.debug != nil {
			rc.debug.LogReferrerCheck(referrer, false, "")
		}
		
		return result, nil
	}

	// Проверка кеша
	if result := rc.getCachedResult(referrer); result != nil {
		rc.incrementCacheHits()
		if rc.metrics != nil {
			rc.metrics.IncrementCacheHits()
		}
		
		if rc.debug != nil {
			rc.debug.LogReferrerCheck(referrer, result.IsFromSearch, result.MatchedPattern)
		}
		
		return result, nil
	}

	if rc.metrics != nil {
		rc.metrics.IncrementCacheMisses()
	}

	// Выполнение проверки
	result := rc.performCheck(referrer)
	
	// Сохранение в кеш
	rc.setCachedResult(referrer, result)
	
	// Логирование для дебага
	if rc.debug != nil {
		rc.debug.LogReferrerCheck(referrer, result.IsFromSearch, result.MatchedPattern)
	}

	// Обновление статистики
	if result.IsFromSearch {
		rc.incrementValidReferrers()
		if result.SearchEngine != "" {
			rc.incrementSearchEngineHit(result.SearchEngine)
		}
	} else {
		rc.incrementInvalidReferrers()
	}

	return result, nil
}

// performCheck выполняет основную проверку referrer
func (rc *ReferrerChecker) performCheck(referrer string) *ReferrerResult {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	// Парсинг URL
	parsedURL, err := url.Parse(referrer)
	if err != nil {
		rc.incrementMalformedURLs()
		return &ReferrerResult{
			IsFromSearch:    false,
			ReferrerType:    ReferrerTypeMalformed,
			OriginalURL:     referrer,
			Confidence:      1.0,
			Timestamp:       time.Now(),
		}
	}

	hostname := strings.ToLower(parsedURL.Hostname())
	if hostname == "" {
		return &ReferrerResult{
			IsFromSearch:    false,
			ReferrerType:    ReferrerTypeMalformed,
			OriginalURL:     referrer,
			Confidence:      1.0,
			Timestamp:       time.Now(),
		}
	}

	// 1. Проверка точных доменов (самый быстрый)
	if rc.exactDomains[hostname] {
		searchEngine := rc.identifySearchEngine(hostname)
		queryParams := rc.extractQueryParameters(parsedURL)
		
		return &ReferrerResult{
			IsFromSearch:     true,
			SearchEngine:     searchEngine,
			Domain:           hostname,
			OriginalURL:      referrer,
			MatchedPattern:   hostname,
			Confidence:       1.0,
			ReferrerType:     ReferrerTypeSearchEngine,
			QueryParameters:  queryParams,
			Timestamp:        time.Now(),
		}
	}

	// 2. Проверка wildcard доменов
	for _, pattern := range rc.wildcardDomains {
		if rc.matchWildcard(hostname, pattern) {
			searchEngine := rc.identifySearchEngine(hostname)
			queryParams := rc.extractQueryParameters(parsedURL)
			
			return &ReferrerResult{
				IsFromSearch:     true,
				SearchEngine:     searchEngine,
				Domain:           hostname,
				OriginalURL:      referrer,
				MatchedPattern:   pattern,
				Confidence:       0.9,
				ReferrerType:     ReferrerTypeSearchEngine,
				QueryParameters:  queryParams,
				Timestamp:        time.Now(),
			}
		}
	}

	// 3. Проверка regex паттернов (самый медленный)
	for _, regex := range rc.compiledPatterns {
		if regex.MatchString(hostname) {
			searchEngine := rc.identifySearchEngine(hostname)
			queryParams := rc.extractQueryParameters(parsedURL)
			
			return &ReferrerResult{
				IsFromSearch:     true,
				SearchEngine:     searchEngine,
				Domain:           hostname,
				OriginalURL:      referrer,
				MatchedPattern:   regex.String(),
				Confidence:       0.8,
				ReferrerType:     ReferrerTypeSearchEngine,
				QueryParameters:  queryParams,
				Timestamp:        time.Now(),
			}
		}
	}

	// 4. Определение типа неизвестного referrer
	referrerType := rc.classifyUnknownReferrer(hostname, parsedURL)
	
	return &ReferrerResult{
		IsFromSearch:    false,
		Domain:          hostname,
		OriginalURL:     referrer,
		Confidence:      0.9,
		ReferrerType:    referrerType,
		QueryParameters: rc.extractQueryParameters(parsedURL),
		Timestamp:       time.Now(),
	}
}

// matchWildcard проверяет соответствие hostname wildcard паттерну
func (rc *ReferrerChecker) matchWildcard(hostname, pattern string) bool {
	// Простое сопоставление с wildcard
	if pattern == "*" {
		return true
	}
	
	if strings.HasPrefix(pattern, "*.") {
		// Паттерн вида *.google.com
		suffix := pattern[2:]
		return hostname == suffix || strings.HasSuffix(hostname, "."+suffix)
	}
	
	if strings.HasSuffix(pattern, ".*") {
		// Паттерн вида google.*
		prefix := pattern[:len(pattern)-2]
		return hostname == prefix || strings.HasPrefix(hostname, prefix+".")
	}
	
	// Общий wildcard matching
	return rc.simpleWildcardMatch(hostname, pattern)
}

// simpleWildcardMatch простое сопоставление с wildcard
func (rc *ReferrerChecker) simpleWildcardMatch(text, pattern string) bool {
	// Разбиваем паттерн по *
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return text == pattern
	}
	
	// Проверяем что текст начинается с первой части
	if !strings.HasPrefix(text, parts[0]) {
		return false
	}
	
	// Проверяем что текст заканчивается последней частью
	if !strings.HasSuffix(text, parts[len(parts)-1]) {
		return false
	}
	
	// Проверяем средние части
	searchText := text[len(parts[0]) : len(text)-len(parts[len(parts)-1])]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(searchText, parts[i])
		if idx == -1 {
			return false
		}
		searchText = searchText[idx+len(parts[i]):]
	}
	
	return true
}

// identifySearchEngine определяет поисковую систему по домену
func (rc *ReferrerChecker) identifySearchEngine(hostname string) string {
	searchEngines := map[string]string{
		// Google
		"google.com": "Google", "google.ru": "Google", "google.de": "Google",
		"google.fr": "Google", "google.co.uk": "Google", "google.it": "Google",
		"google.es": "Google", "google.ca": "Google", "google.com.au": "Google",
		"google.co.jp": "Google", "google.co.kr": "Google", "google.com.br": "Google",
		
		// Bing
		"bing.com": "Bing", "msn.com": "Bing", "live.com": "Bing",
		
		// Yandex
		"yandex.ru": "Yandex", "yandex.com": "Yandex", "yandex.ua": "Yandex",
		"yandex.by": "Yandex", "yandex.kz": "Yandex", "ya.ru": "Yandex",
		
		// Other search engines
		"duckduckgo.com": "DuckDuckGo", "yahoo.com": "Yahoo", "search.yahoo.com": "Yahoo",
		"baidu.com": "Baidu", "sogou.com": "Sogou", "so.com": "360 Search",
		"ask.com": "Ask", "aol.com": "AOL", "ecosia.org": "Ecosia",
		"startpage.com": "Startpage", "searx.me": "SearX",
	}
	
	// Точное совпадение
	if engine, exists := searchEngines[hostname]; exists {
		return engine
	}
	
	// Проверка поддоменов
	for domain, engine := range searchEngines {
		if strings.HasSuffix(hostname, "."+domain) {
			return engine
		}
	}
	
	// Попытка определить по ключевым словам
	if strings.Contains(hostname, "google") {
		return "Google"
	}
	if strings.Contains(hostname, "bing") || strings.Contains(hostname, "msn") {
		return "Bing"
	}
	if strings.Contains(hostname, "yandex") {
		return "Yandex"
	}
	if strings.Contains(hostname, "yahoo") {
		return "Yahoo"
	}
	if strings.Contains(hostname, "baidu") {
		return "Baidu"
	}
	
	return "Unknown Search Engine"
}

// extractQueryParameters извлекает параметры запроса из URL
func (rc *ReferrerChecker) extractQueryParameters(parsedURL *url.URL) map[string]string {
	params := make(map[string]string)
	
	// Извлекаем основные параметры поиска
	queryParams := []string{"q", "query", "search", "p", "text", "wd", "w", "s"}
	
	for _, param := range queryParams {
		if value := parsedURL.Query().Get(param); value != "" {
			params[param] = value
		}
	}
	
	// Дополнительные полезные параметры
	additionalParams := []string{"hl", "gl", "lr", "ie", "oe", "safe", "tbm"}
	for _, param := range additionalParams {
		if value := parsedURL.Query().Get(param); value != "" {
			params[param] = value
		}
	}
	
	return params
}

// classifyUnknownReferrer классифицирует неизвестный referrer
func (rc *ReferrerChecker) classifyUnknownReferrer(hostname string, parsedURL *url.URL) ReferrerType {
	// Социальные сети
	socialDomains := []string{
		"facebook.com", "twitter.com", "x.com", "instagram.com", "linkedin.com",
		"pinterest.com", "reddit.com", "tiktok.com", "snapchat.com", "whatsapp.com",
		"telegram.org", "vk.com", "ok.ru", "youtube.com", "twitch.tv",
	}
	
	for _, domain := range socialDomains {
		if hostname == domain || strings.HasSuffix(hostname, "."+domain) {
			return ReferrerTypeSocialMedia
		}
	}
	
	// Проверка на внутренние ссылки (если это тот же домен)
	// Примечание: для полной проверки нужен текущий домен сайта
	
	// По умолчанию - прямая ссылка
	return ReferrerTypeDirectLink
}

// getCachedResult получает результат из кеша
func (rc *ReferrerChecker) getCachedResult(referrer string) *ReferrerResult {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()
	
	if result, exists := rc.cache[referrer]; exists {
		// Проверка TTL
		if time.Since(result.Timestamp) < rc.cacheTTL {
			return result
		}
		// Удаление устаревшей записи
		delete(rc.cache, referrer)
	}
	
	return nil
}

// setCachedResult сохраняет результат в кеш
func (rc *ReferrerChecker) setCachedResult(referrer string, result *ReferrerResult) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	
	// Проверка размера кеша
	if len(rc.cache) >= rc.maxCache {
		rc.cleanupCache()
	}
	
	rc.cache[referrer] = result
	
	if rc.debug != nil {
		rc.debug.LogCacheOperation(&CacheDebugInfo{
			Key:       referrer,
			Operation: "set",
			Hit:       false,
			Value:     result,
			TTL:       rc.cacheTTL,
			Timestamp: time.Now(),
		})
	}
}

// cleanupCache очищает старые записи из кеша
func (rc *ReferrerChecker) cleanupCache() {
	now := time.Now()
	
	for key, result := range rc.cache {
		if now.Sub(result.Timestamp) > rc.cacheTTL {
			delete(rc.cache, key)
		}
	}
	
	// Если кеш все еще переполнен, удаляем самые старые записи
	if len(rc.cache) >= rc.maxCache {
		count := 0
		target := len(rc.cache) / 2
		
		for key := range rc.cache {
			if count >= target {
				break
			}
			delete(rc.cache, key)
			count++
		}
	}
}

// AddDomain добавляет новый разрешенный домен в runtime
func (rc *ReferrerChecker) AddDomain(domain string) error {
	if domain == "" {
		return nil
	}
	
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	
	// Добавляем в список доменов
	rc.allowedDomains = append(rc.allowedDomains, domain)
	
	// Классифицируем и добавляем в соответствующую структуру
	if rc.isExactDomain(domain) {
		rc.exactDomains[strings.ToLower(domain)] = true
	} else if rc.isWildcardDomain(domain) {
		rc.wildcardDomains = append(rc.wildcardDomains, strings.ToLower(domain))
	} else {
		pattern := rc.convertToRegex(domain)
		if regex, err := regexp.Compile("(?i)" + pattern); err == nil {
			rc.compiledPatterns = append(rc.compiledPatterns, regex)
		} else {
			return err
		}
	}
	
	// Очищаем кеш после добавления нового домена
	rc.cache = make(map[string]*ReferrerResult)
	
	rc.logger.Info("added new referrer domain",
		zap.String("domain", domain),
		zap.Int("total_domains", len(rc.allowedDomains)),
	)
	
	return nil
}

// RemoveDomain удаляет домен
func (rc *ReferrerChecker) RemoveDomain(domain string) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	
	// Удаляем из основного списка
	for i, d := range rc.allowedDomains {
		if d == domain {
			rc.allowedDomains = append(rc.allowedDomains[:i], rc.allowedDomains[i+1:]...)
			break
		}
	}
	
	// Переинициализируем все структуры
	domains := make([]string, len(rc.allowedDomains))
	copy(domains, rc.allowedDomains)
	
	rc.initializePatterns(domains)
	
	rc.logger.Info("removed referrer domain",
		zap.String("domain", domain),
		zap.Int("total_domains", len(rc.allowedDomains)),
	)
}

// GetStats возвращает статистику
func (rc *ReferrerChecker) GetStats() map[string]interface{} {
	if !rc.enabled {
		return map[string]interface{}{"enabled": false}
	}

	rc.mutex.RLock()
	defer rc.mutex.RUnlock()
	
	hitRate := 0.0
	if rc.totalChecks > 0 {
		hitRate = float64(rc.cacheHits) / float64(rc.totalChecks)
	}
	
	validRate := 0.0
	if rc.totalChecks > 0 {
		validRate = float64(rc.validReferrers) / float64(rc.totalChecks)
	}
	
	stats := map[string]interface{}{
		"enabled":             true,
		"total_domains":       len(rc.allowedDomains),
		"exact_domains":       len(rc.exactDomains),
		"wildcard_domains":    len(rc.wildcardDomains),
		"regex_patterns":      len(rc.compiledPatterns),
		"cache_size":          len(rc.cache),
		"cache_max_size":      rc.maxCache,
		"total_checks":        rc.totalChecks,
		"valid_referrers":     rc.validReferrers,
		"invalid_referrers":   rc.invalidReferrers,
		"empty_referrers":     rc.emptyReferrers,
		"cache_hits":          rc.cacheHits,
		"malformed_urls":      rc.malformedURLs,
		"cache_hit_rate":      hitRate,
		"valid_rate":          validRate,
		"search_engine_hits":  rc.searchEngineHits,
	}
	
	return stats
}

// ClearCache очищает кеш
func (rc *ReferrerChecker) ClearCache() {
	if !rc.enabled {
		return
	}
	
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	
	rc.cache = make(map[string]*ReferrerResult)
	rc.logger.Info("referrer checker cache cleared")
}

// Методы для статистики
func (rc *ReferrerChecker) incrementTotalChecks() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.totalChecks++
}

func (rc *ReferrerChecker) incrementValidReferrers() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.validReferrers++
}

func (rc *ReferrerChecker) incrementInvalidReferrers() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.invalidReferrers++
}

func (rc *ReferrerChecker) incrementEmptyReferrers() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.emptyReferrers++
}

func (rc *ReferrerChecker) incrementCacheHits() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.cacheHits++
}

func (rc *ReferrerChecker) incrementMalformedURLs() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.malformedURLs++
}

func (rc *ReferrerChecker) incrementSearchEngineHit(searchEngine string) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.searchEngineHits[searchEngine]++
}