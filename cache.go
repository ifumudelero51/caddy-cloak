package botredirect

import (
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Cache универсальная система кеширования для всех компонентов
type Cache struct {
	// Основное хранилище
	store map[string]*CacheEntry
	mutex sync.RWMutex

	// Конфигурация
	ttl             time.Duration
	maxSize         int
	cleanupInterval time.Duration

	// Статистика (используем atomic для thread-safety)
	hits      int64
	misses    int64
	evictions int64

	// Компоненты
	metrics *Metrics
	debug   *DebugConfig
	logger  *zap.Logger

	// Очистка
	stopCleanup chan bool
	isRunning   bool
	cleanupOnce sync.Once
}

// CacheEntry запись в кеше с метаданными
type CacheEntry struct {
	Key        string
	Value      interface{}
	CreatedAt  time.Time
	LastAccess time.Time
	TTL        time.Duration
	HitCount   int64
}

// CacheStats статистика кеша
type CacheStats struct {
	Size      int
	Hits      int64
	Misses    int64
	Evictions int64
	HitRate   float64
}

// NewCache создает новый экземпляр кеша
func NewCache(config *Config, metrics *Metrics, debug *DebugConfig, logger *zap.Logger) *Cache {
	cache := &Cache{
		store:           make(map[string]*CacheEntry),
		ttl:             config.CacheTTL,
		maxSize:         config.MaxCacheSize,
		cleanupInterval: config.CleanupInterval,
		metrics:         metrics,
		debug:           debug,
		logger:          logger,
		stopCleanup:     make(chan bool, 1), // буферизованный канал
		isRunning:       false,
	}

	// Запускаем фоновую очистку
	cache.startCleanup()

	logger.Info("cache system initialized",
		zap.Duration("ttl", cache.ttl),
		zap.Int("max_size", cache.maxSize),
		zap.Duration("cleanup_interval", cache.cleanupInterval),
	)

	return cache
}

// Get получает значение из кеша
func (c *Cache) Get(key string) interface{} {
	c.mutex.RLock()
	entry, exists := c.store[key]
	c.mutex.RUnlock()

	if !exists {
		c.incrementMisses()
		if c.debug != nil {
			c.debug.LogCacheOperation(&CacheDebugInfo{
				Key:       key,
				Operation: "miss",
				Hit:       false,
				Timestamp: time.Now(),
			})
		}
		return nil
	}

	// Проверяем TTL
	if c.isExpired(entry) {
		c.Delete(key)
		c.incrementMisses()
		return nil
	}

	// Обновляем статистику доступа
	c.mutex.Lock()
	entry.LastAccess = time.Now()
	atomic.AddInt64(&entry.HitCount, 1)
	c.mutex.Unlock()

	c.incrementHits()
	if c.debug != nil {
		c.debug.LogCacheOperation(&CacheDebugInfo{
			Key:       key,
			Operation: "hit",
			Hit:       true,
			Value:     entry.Value,
			TTL:       entry.TTL,
			Timestamp: time.Now(),
		})
	}

	return entry.Value
}

// Set сохраняет значение в кеш
func (c *Cache) Set(key string, value interface{}) {
	c.SetWithTTL(key, value, c.ttl)
}

// SetWithTTL сохраняет значение с кастомным TTL
func (c *Cache) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Проверяем размер кеша
	if len(c.store) >= c.maxSize {
		c.evictLRUUnsafe() // unsafe версия для использования под мьютексом
	}

	entry := &CacheEntry{
		Key:        key,
		Value:      value,
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
		TTL:        ttl,
		HitCount:   0,
	}

	c.store[key] = entry

	if c.debug != nil {
		c.debug.LogCacheOperation(&CacheDebugInfo{
			Key:       key,
			Operation: "set",
			Hit:       false,
			Value:     value,
			TTL:       ttl,
			Timestamp: time.Now(),
		})
	}
}

// Delete удаляет запись из кеша
func (c *Cache) Delete(key string) {
	c.mutex.Lock()
	delete(c.store, key)
	c.mutex.Unlock()

	if c.debug != nil {
		c.debug.LogCacheOperation(&CacheDebugInfo{
			Key:       key,
			Operation: "delete",
			Hit:       false,
			Timestamp: time.Now(),
		})
	}
}

// Clear очищает весь кеш
func (c *Cache) Clear() {
	c.mutex.Lock()
	c.store = make(map[string]*CacheEntry)
	c.mutex.Unlock()

	c.logger.Info("cache cleared")
}

// isExpired проверяет истек ли TTL записи
func (c *Cache) isExpired(entry *CacheEntry) bool {
	if entry.TTL <= 0 {
		return false // Бесконечный TTL
	}
	return time.Since(entry.CreatedAt) > entry.TTL
}

// evictLRUUnsafe удаляет наименее используемую запись (вызывать под мьютексом)
func (c *Cache) evictLRUUnsafe() {
	var oldestKey string
	oldestTime := time.Now()

	for key, entry := range c.store {
		if entry.LastAccess.Before(oldestTime) {
			oldestTime = entry.LastAccess
			oldestKey = key
		}
	}

	if oldestKey != "" {
		delete(c.store, oldestKey)
		c.incrementEvictions()
	}
}

// cleanup удаляет устаревшие записи
func (c *Cache) cleanup() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	keysToDelete := make([]string, 0)

	for key, entry := range c.store {
		if c.isExpired(entry) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(c.store, key)
	}

	// ИСПРАВЛЕНИЕ: Принудительная очистка если кеш переполнен
	if len(c.store) > c.maxSize {
		c.evictLRUUnsafe()
	}

	if len(keysToDelete) > 0 {
		c.logger.Debug("cache cleanup completed",
			zap.Int("expired_entries", len(keysToDelete)),
			zap.Int("current_size", len(c.store)),
		)
	}
}

// startCleanup запускает фоновую очистку кеша
func (c *Cache) startCleanup() {
	c.cleanupOnce.Do(func() {
		c.isRunning = true
		go func() {
			ticker := time.NewTicker(c.cleanupInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					c.cleanup()
				case <-c.stopCleanup:
					c.isRunning = false
					return
				}
			}
		}()
	})
}

// StopCleanup останавливает фоновую очистку
func (c *Cache) StopCleanup() {
	if c.isRunning {
		select {
		case c.stopCleanup <- true:
		default:
		}
	}
}

// GetStats возвращает статистику кеша
func (c *Cache) GetStats() *CacheStats {
	c.mutex.RLock()
	size := len(c.store)
	c.mutex.RUnlock()

	hits := atomic.LoadInt64(&c.hits)
	misses := atomic.LoadInt64(&c.misses)
	evictions := atomic.LoadInt64(&c.evictions)

	hitRate := 0.0
	totalRequests := hits + misses
	if totalRequests > 0 {
		hitRate = float64(hits) / float64(totalRequests)
	}

	return &CacheStats{
		Size:      size,
		Hits:      hits,
		Misses:    misses,
		Evictions: evictions,
		HitRate:   hitRate,
	}
}

// UpdateMetrics обновляет метрики в системе мониторинга
func (c *Cache) UpdateMetrics() {
	if c.metrics != nil {
		stats := c.GetStats()
		c.metrics.SetCacheSize(int64(stats.Size))

		if c.debug != nil && c.debug.VerboseMetrics {
			c.debug.LogCacheStats(stats.Size, stats.Hits, stats.Misses, stats.HitRate)
		}
	}
}

// Методы для статистики (используем atomic operations)
func (c *Cache) incrementHits() {
	atomic.AddInt64(&c.hits, 1)
}

func (c *Cache) incrementMisses() {
	atomic.AddInt64(&c.misses, 1)
}

func (c *Cache) incrementEvictions() {
	atomic.AddInt64(&c.evictions, 1)
}
