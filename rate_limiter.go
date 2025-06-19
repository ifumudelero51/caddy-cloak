package botredirect

import (
	"net"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TokenBucket реализует алгоритм token bucket для rate limiting
type TokenBucket struct {
	capacity   int
	tokens     int
	refillRate int
	lastRefill time.Time
	mutex      sync.Mutex
}

// RateLimiter управляет rate limiting для различных IP адресов
type RateLimiter struct {
	// Основные настройки
	enabled        bool
	maxRequests    int
	maxDNSRequests int
	window         time.Duration

	// Хранилища для rate limiting
	requestBuckets map[string]*TokenBucket
	dnsBuckets     map[string]*TokenBucket

	// Мьютексы для безопасного доступа
	requestMutex sync.RWMutex
	dnsMutex     sync.RWMutex

	// Очистка старых записей
	cleanupInterval time.Duration
	lastCleanup     time.Time
	cleanupMutex    sync.Mutex

	// Управление горутиной очистки
	stopCleanup    chan bool
	cleanupRunning bool

	// Метрики и логирование
	metrics *Metrics
	logger  *zap.Logger
}

// NewRateLimiter создает новый экземпляр rate limiter
func NewRateLimiter(config *Config, metrics *Metrics, logger *zap.Logger) *RateLimiter {
	if !config.EnableRateLimit {
		return &RateLimiter{enabled: false}
	}

	rl := &RateLimiter{
		enabled:         true,
		maxRequests:     config.MaxRequestsPerIP,
		maxDNSRequests:  config.MaxDNSPerSecond,
		window:          config.RateLimitWindow,
		requestBuckets:  make(map[string]*TokenBucket),
		dnsBuckets:      make(map[string]*TokenBucket),
		cleanupInterval: 5 * time.Minute,
		lastCleanup:     time.Now(),
		stopCleanup:     make(chan bool, 1), // буферизованный канал
		cleanupRunning:  false,
		metrics:         metrics,
		logger:          logger,
	}

	// Запускаем горутину для периодической очистки
	rl.startCleanupRoutine()

	logger.Info("rate limiter initialized",
		zap.Bool("enabled", true),
		zap.Int("max_requests_per_ip", config.MaxRequestsPerIP),
		zap.Int("max_dns_per_second", config.MaxDNSPerSecond),
		zap.Duration("window", config.RateLimitWindow),
	)

	return rl
}

// CheckRequest проверяет, разрешен ли запрос от данного IP
func (rl *RateLimiter) CheckRequest(clientIP string) bool {
	if !rl.enabled {
		return true
	}

	// Извлекаем IP из адреса (убираем порт)
	ip := rl.extractIP(clientIP)

	allowed := rl.checkTokenBucket(ip, rl.maxRequests, &rl.requestMutex, rl.requestBuckets)

	if !allowed && rl.metrics != nil {
		rl.metrics.IncrementRateLimitBlocked()
		rl.logger.Warn("request rate limited",
			zap.String("ip", ip),
			zap.Int("max_requests", rl.maxRequests),
		)
	}

	return allowed
}

// CheckDNSRequest проверяет, разрешен ли DNS запрос от данного IP
func (rl *RateLimiter) CheckDNSRequest(clientIP string) bool {
	if !rl.enabled {
		return true
	}

	ip := rl.extractIP(clientIP)

	allowed := rl.checkTokenBucket(ip, rl.maxDNSRequests, &rl.dnsMutex, rl.dnsBuckets)

	if !allowed && rl.metrics != nil {
		rl.metrics.IncrementRateLimited()
		rl.logger.Warn("DNS request rate limited",
			zap.String("ip", ip),
			zap.Int("max_dns_requests", rl.maxDNSRequests),
		)
	}

	return allowed
}

// checkTokenBucket проверяет и обновляет token bucket для данного IP
func (rl *RateLimiter) checkTokenBucket(ip string, maxRate int, mutex *sync.RWMutex, buckets map[string]*TokenBucket) bool {
	mutex.Lock()
	defer mutex.Unlock()

	bucket, exists := buckets[ip]
	if !exists {
		// Создаем новый bucket для IP
		bucket = &TokenBucket{
			capacity:   maxRate,
			tokens:     maxRate,
			refillRate: maxRate,
			lastRefill: time.Now(),
		}
		buckets[ip] = bucket
	}

	return bucket.allowRequest()
}

// allowRequest проверяет, можно ли выполнить запрос (потребляет один токен)
func (tb *TokenBucket) allowRequest() bool {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	// Пополняем токены на основе прошедшего времени
	tb.refill()

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}

// refill пополняет токены в bucket на основе прошедшего времени
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	if elapsed >= time.Second {
		// Добавляем токены за каждую секунду
		seconds := int(elapsed.Seconds())
		tokensToAdd := seconds * tb.refillRate

		tb.tokens += tokensToAdd
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}

		tb.lastRefill = now
	}
}

// extractIP извлекает IP адрес из строки адреса (убирает порт)
func (rl *RateLimiter) extractIP(address string) string {
	// Обработка IPv6 адресов с портом [::1]:8080
	if strings.HasPrefix(address, "[") {
		end := strings.Index(address, "]")
		if end != -1 {
			return address[1:end]
		}
	}

	host, _, err := net.SplitHostPort(address)
	if err != nil {
		// Если не удалось разделить, возвращаем как есть
		return address
	}
	return host
}

// startCleanupRoutine запускает горутину для периодической очистки старых записей
func (rl *RateLimiter) startCleanupRoutine() {
	rl.cleanupMutex.Lock()
	defer rl.cleanupMutex.Unlock()

	if rl.cleanupRunning {
		return
	}

	rl.cleanupRunning = true
	go func() {
		ticker := time.NewTicker(rl.cleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				rl.cleanup()
			case <-rl.stopCleanup:
				rl.cleanupMutex.Lock()
				rl.cleanupRunning = false
				rl.cleanupMutex.Unlock()
				return
			}
		}
	}()
}

// cleanup удаляет старые неиспользуемые bucket'ы
func (rl *RateLimiter) cleanup() {
	now := time.Now()
	cutoff := now.Add(-rl.window * 2) // Удаляем bucket'ы старше 2 окон

	// Очистка request buckets
	rl.requestMutex.Lock()
	for ip, bucket := range rl.requestBuckets {
		bucket.mutex.Lock()
		lastRefill := bucket.lastRefill
		bucket.mutex.Unlock()

		if lastRefill.Before(cutoff) {
			delete(rl.requestBuckets, ip)
		}
	}
	requestCount := len(rl.requestBuckets)
	rl.requestMutex.Unlock()

	// Очистка DNS buckets
	rl.dnsMutex.Lock()
	for ip, bucket := range rl.dnsBuckets {
		bucket.mutex.Lock()
		lastRefill := bucket.lastRefill
		bucket.mutex.Unlock()

		if lastRefill.Before(cutoff) {
			delete(rl.dnsBuckets, ip)
		}
	}
	dnsCount := len(rl.dnsBuckets)
	rl.dnsMutex.Unlock()

	rl.logger.Debug("rate limiter cleanup completed",
		zap.Int("active_request_buckets", requestCount),
		zap.Int("active_dns_buckets", dnsCount),
	)
}

// GetStats возвращает статистику rate limiter
func (rl *RateLimiter) GetStats() map[string]interface{} {
	if !rl.enabled {
		return map[string]interface{}{"enabled": false}
	}

	rl.requestMutex.RLock()
	requestBuckets := len(rl.requestBuckets)
	rl.requestMutex.RUnlock()

	rl.dnsMutex.RLock()
	dnsBuckets := len(rl.dnsBuckets)
	rl.dnsMutex.RUnlock()

	return map[string]interface{}{
		"enabled":                true,
		"max_requests_per_ip":    rl.maxRequests,
		"max_dns_per_second":     rl.maxDNSRequests,
		"window_seconds":         rl.window.Seconds(),
		"active_request_buckets": requestBuckets,
		"active_dns_buckets":     dnsBuckets,
	}
}

// IsEnabled возвращает статус включенности rate limiter
func (rl *RateLimiter) IsEnabled() bool {
	return rl.enabled
}

// UpdateLimits обновляет лимиты rate limiter (для runtime конфигурации)
func (rl *RateLimiter) UpdateLimits(maxRequests, maxDNS int, window time.Duration) {
	if !rl.enabled {
		return
	}

	rl.requestMutex.Lock()
	rl.maxRequests = maxRequests
	rl.requestMutex.Unlock()

	rl.dnsMutex.Lock()
	rl.maxDNSRequests = maxDNS
	rl.window = window
	rl.dnsMutex.Unlock()

	rl.logger.Info("rate limiter limits updated",
		zap.Int("max_requests_per_ip", maxRequests),
		zap.Int("max_dns_per_second", maxDNS),
		zap.Duration("window", window),
	)
}

// Reset сбрасывает все bucket'ы (для тестирования или экстренных случаев)
func (rl *RateLimiter) Reset() {
	if !rl.enabled {
		return
	}

	rl.requestMutex.Lock()
	rl.requestBuckets = make(map[string]*TokenBucket)
	rl.requestMutex.Unlock()

	rl.dnsMutex.Lock()
	rl.dnsBuckets = make(map[string]*TokenBucket)
	rl.dnsMutex.Unlock()

	rl.logger.Info("rate limiter reset completed")
}

// Shutdown останавливает rate limiter
func (rl *RateLimiter) Shutdown() {
	if !rl.enabled {
		return
	}

	rl.cleanupMutex.Lock()
	if rl.cleanupRunning {
		select {
		case rl.stopCleanup <- true:
		default:
		}
	}
	rl.cleanupMutex.Unlock()
}
