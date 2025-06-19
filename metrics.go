package botredirect

import (
	"expvar"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Metrics содержит все метрики плагина
type Metrics struct {
	// Основные счетчики
	BotRequests        *expvar.Int
	SearchUserRequests *expvar.Int
	DirectUserRequests *expvar.Int
	
	// Метрики кеша
	CacheHits          *expvar.Int
	CacheMisses        *expvar.Int
	CacheSize          *expvar.Int
	
	// Метрики DNS
	DNSRequests        *expvar.Int
	DNSTimeouts        *expvar.Int
	DNSErrors          *expvar.Int
	DNSSuccesses       *expvar.Int
	
	// Метрики rate limiting
	RateLimited        *expvar.Int
	RateLimitBlocked   *expvar.Int
	
	// Метрики производительности
	TotalRequests      *expvar.Int
	ProcessingTime     *expvar.Float
	AverageResponseTime *expvar.Float
	
	// Детальные метрики (если включены)
	UserAgentChecks    *expvar.Int
	IPRangeChecks      *expvar.Int
	ReferrerChecks     *expvar.Int
	
	// Внутренние поля
	enabled        bool
	verbose        bool
	startTime      time.Time
	mutex          sync.RWMutex
	responseTimes  []float64
	maxSamples     int
	logger         *zap.Logger
	
	// Atomic counters для thread-safe операций
	responseTimeSum   int64 // в наносекундах
	responseTimeCount int64
}

// NewMetrics создает новый экземпляр метрик
func NewMetrics(enabled, verbose bool, logger *zap.Logger) *Metrics {
	if !enabled {
		return &Metrics{enabled: false}
	}

	m := &Metrics{
		enabled:   true,
		verbose:   verbose,
		startTime: time.Now(),
		maxSamples: 1000, // Максимум 1000 образцов для среднего времени ответа
		logger:    logger,
	}

	// Инициализация expvar метрик
	m.BotRequests = expvar.NewInt("bot_redirect.bot_requests")
	m.SearchUserRequests = expvar.NewInt("bot_redirect.search_user_requests")
	m.DirectUserRequests = expvar.NewInt("bot_redirect.direct_user_requests")
	
	m.CacheHits = expvar.NewInt("bot_redirect.cache_hits")
	m.CacheMisses = expvar.NewInt("bot_redirect.cache_misses")
	m.CacheSize = expvar.NewInt("bot_redirect.cache_size")
	
	m.DNSRequests = expvar.NewInt("bot_redirect.dns_requests")
	m.DNSTimeouts = expvar.NewInt("bot_redirect.dns_timeouts")
	m.DNSErrors = expvar.NewInt("bot_redirect.dns_errors")
	m.DNSSuccesses = expvar.NewInt("bot_redirect.dns_successes")
	
	m.RateLimited = expvar.NewInt("bot_redirect.rate_limited")
	m.RateLimitBlocked = expvar.NewInt("bot_redirect.rate_limit_blocked")
	
	m.TotalRequests = expvar.NewInt("bot_redirect.total_requests")
	m.ProcessingTime = expvar.NewFloat("bot_redirect.processing_time_ms")
	m.AverageResponseTime = expvar.NewFloat("bot_redirect.avg_response_time_ms")

	if verbose {
		m.UserAgentChecks = expvar.NewInt("bot_redirect.user_agent_checks")
		m.IPRangeChecks = expvar.NewInt("bot_redirect.ip_range_checks")
		m.ReferrerChecks = expvar.NewInt("bot_redirect.referrer_checks")
	}

	// Регистрация дополнительных метрик
	expvar.Publish("bot_redirect.uptime_seconds", expvar.Func(func() interface{} {
		return time.Since(m.startTime).Seconds()
	}))

	expvar.Publish("bot_redirect.cache_hit_rate", expvar.Func(func() interface{} {
		return m.getCacheHitRate()
	}))

	expvar.Publish("bot_redirect.dns_success_rate", expvar.Func(func() interface{} {
		return m.getDNSSuccessRate()
	}))

	logger.Info("metrics system initialized",
		zap.Bool("enabled", enabled),
		zap.Bool("verbose", verbose),
	)

	return m
}

// IncrementBotRequests увеличивает счетчик запросов от ботов
func (m *Metrics) IncrementBotRequests() {
	if !m.enabled {
		return
	}
	m.BotRequests.Add(1)
	m.TotalRequests.Add(1)
}

// IncrementSearchUserRequests увеличивает счетчик запросов от пользователей с поисковиков
func (m *Metrics) IncrementSearchUserRequests() {
	if !m.enabled {
		return
	}
	m.SearchUserRequests.Add(1)
	m.TotalRequests.Add(1)
}

// IncrementDirectUserRequests увеличивает счетчик прямых запросов пользователей
func (m *Metrics) IncrementDirectUserRequests() {
	if !m.enabled {
		return
	}
	m.DirectUserRequests.Add(1)
	m.TotalRequests.Add(1)
}

// IncrementCacheHits увеличивает счетчик попаданий в кеш
func (m *Metrics) IncrementCacheHits() {
	if !m.enabled {
		return
	}
	m.CacheHits.Add(1)
}

// IncrementCacheMisses увеличивает счетчик промахов кеша
func (m *Metrics) IncrementCacheMisses() {
	if !m.enabled {
		return
	}
	m.CacheMisses.Add(1)
}

// SetCacheSize устанавливает текущий размер кеша
func (m *Metrics) SetCacheSize(size int64) {
	if !m.enabled {
		return
	}
	m.CacheSize.Set(size)
}

// IncrementDNSRequests увеличивает счетчик DNS запросов
func (m *Metrics) IncrementDNSRequests() {
	if !m.enabled {
		return
	}
	m.DNSRequests.Add(1)
}

// IncrementDNSTimeouts увеличивает счетчик таймаутов DNS
func (m *Metrics) IncrementDNSTimeouts() {
	if !m.enabled {
		return
	}
	m.DNSTimeouts.Add(1)
}

// IncrementDNSErrors увеличивает счетчик ошибок DNS
func (m *Metrics) IncrementDNSErrors() {
	if !m.enabled {
		return
	}
	m.DNSErrors.Add(1)
}

// IncrementDNSSuccesses увеличивает счетчик успешных DNS запросов
func (m *Metrics) IncrementDNSSuccesses() {
	if !m.enabled {
		return
	}
	m.DNSSuccesses.Add(1)
}

// IncrementRateLimited увеличивает счетчик rate limited запросов
func (m *Metrics) IncrementRateLimited() {
	if !m.enabled {
		return
	}
	m.RateLimited.Add(1)
}

// IncrementRateLimitBlocked увеличивает счетчик заблокированных запросов
func (m *Metrics) IncrementRateLimitBlocked() {
	if !m.enabled {
		return
	}
	m.RateLimitBlocked.Add(1)
}

// RecordProcessingTime записывает время обработки запроса
func (m *Metrics) RecordProcessingTime(duration time.Duration) {
	if !m.enabled {
		return
	}
	
	durationMs := float64(duration.Nanoseconds()) / 1e6
	m.ProcessingTime.Set(durationMs)
	
	// ИСПРАВЛЕНИЕ: Безопасная работа с atomic операциями
	atomic.AddInt64(&m.responseTimeSum, duration.Nanoseconds())
	count := atomic.AddInt64(&m.responseTimeCount, 1)
	
	// Вычисляем среднее используя atomic значения
	avgNs := atomic.LoadInt64(&m.responseTimeSum) / count
	avgMs := float64(avgNs) / 1e6
	m.AverageResponseTime.Set(avgMs)
	
	// Обновляем слайс для детальной статистики (под мьютексом)
	m.mutex.Lock()
	if len(m.responseTimes) >= m.maxSamples {
		// Сдвигаем слайс вместо переаллокации
		copy(m.responseTimes, m.responseTimes[1:])
		m.responseTimes = m.responseTimes[:len(m.responseTimes)-1]
	}
	m.responseTimes = append(m.responseTimes, durationMs)
	m.mutex.Unlock()
}

// Методы для детальных метрик (только если verbose=true)

// IncrementUserAgentChecks увеличивает счетчик проверок User-Agent
func (m *Metrics) IncrementUserAgentChecks() {
	if !m.enabled || !m.verbose || m.UserAgentChecks == nil {
		return
	}
	m.UserAgentChecks.Add(1)
}

// IncrementIPRangeChecks увеличивает счетчик проверок IP диапазонов
func (m *Metrics) IncrementIPRangeChecks() {
	if !m.enabled || !m.verbose || m.IPRangeChecks == nil {
		return
	}
	m.IPRangeChecks.Add(1)
}

// IncrementReferrerChecks увеличивает счетчик проверок referrer
func (m *Metrics) IncrementReferrerChecks() {
	if !m.enabled || !m.verbose || m.ReferrerChecks == nil {
		return
	}
	m.ReferrerChecks.Add(1)
}

// getCacheHitRate вычисляет коэффициент попаданий в кеш
func (m *Metrics) getCacheHitRate() float64 {
	hits := m.CacheHits.Value()
	misses := m.CacheMisses.Value()
	total := hits + misses
	
	if total == 0 {
		return 0.0
	}
	
	return float64(hits) / float64(total)
}

// getDNSSuccessRate вычисляет коэффициент успешных DNS запросов
func (m *Metrics) getDNSSuccessRate() float64 {
	successes := m.DNSSuccesses.Value()
	total := m.DNSRequests.Value()
	
	if total == 0 {
		return 0.0
	}
	
	return float64(successes) / float64(total)
}

// GetStats возвращает сводную статистику
func (m *Metrics) GetStats() map[string]interface{} {
	if !m.enabled {
		return map[string]interface{}{"enabled": false}
	}

	stats := map[string]interface{}{
		"enabled":              true,
		"uptime_seconds":       time.Since(m.startTime).Seconds(),
		"total_requests":       m.TotalRequests.Value(),
		"bot_requests":         m.BotRequests.Value(),
		"search_user_requests": m.SearchUserRequests.Value(),
		"direct_user_requests": m.DirectUserRequests.Value(),
		"cache_hits":           m.CacheHits.Value(),
		"cache_misses":         m.CacheMisses.Value(),
		"cache_hit_rate":       m.getCacheHitRate(),
		"cache_size":           m.CacheSize.Value(),
		"dns_requests":         m.DNSRequests.Value(),
		"dns_timeouts":         m.DNSTimeouts.Value(),
		"dns_errors":           m.DNSErrors.Value(),
		"dns_successes":        m.DNSSuccesses.Value(),
		"dns_success_rate":     m.getDNSSuccessRate(),
		"rate_limited":         m.RateLimited.Value(),
		"rate_limit_blocked":   m.RateLimitBlocked.Value(),
		"avg_response_time_ms": m.AverageResponseTime.Value(),
	}

	if m.verbose {
		if m.UserAgentChecks != nil {
			stats["user_agent_checks"] = m.UserAgentChecks.Value()
		}
		if m.IPRangeChecks != nil {
			stats["ip_range_checks"] = m.IPRangeChecks.Value()
		}
		if m.ReferrerChecks != nil {
			stats["referrer_checks"] = m.ReferrerChecks.Value()
		}
	}

	return stats
}

// LogStats выводит статистику в лог
func (m *Metrics) LogStats() {
	if !m.enabled {
		return
	}

	stats := m.GetStats()
	
	m.logger.Info("bot_redirect metrics",
		zap.Int64("total_requests", m.TotalRequests.Value()),
		zap.Int64("bot_requests", m.BotRequests.Value()),
		zap.Int64("search_user_requests", m.SearchUserRequests.Value()),
		zap.Int64("direct_user_requests", m.DirectUserRequests.Value()),
		zap.Float64("cache_hit_rate", m.getCacheHitRate()),
		zap.Float64("dns_success_rate", m.getDNSSuccessRate()),
		zap.Float64("avg_response_time_ms", m.AverageResponseTime.Value()),
	)
	
	if m.verbose {
		m.logger.Debug("bot_redirect detailed metrics",
			zap.Any("all_stats", stats),
		)
	}
}

// ServeHTTP предоставляет HTTP endpoint для метрик
func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !m.enabled {
		http.Error(w, "Metrics disabled", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	
	// Используем встроенный expvar handler
	expvar.Handler().ServeHTTP(w, r)
}

// StartPeriodicLogging запускает периодическое логирование статистики
func (m *Metrics) StartPeriodicLogging(interval time.Duration) {
	if !m.enabled {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			m.LogStats()
		}
	}()

	m.logger.Info("started periodic metrics logging",
		zap.Duration("interval", interval),
	)
}