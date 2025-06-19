package botredirect

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// IPRangeChecker отвечает за проверку IP-адресов на принадлежность к диапазонам ботов
type IPRangeChecker struct {
	// IP сети и диапазоны
	ipv4Networks []*net.IPNet
	ipv6Networks []*net.IPNet
	
	// Отдельные IP адреса для быстрой проверки
	singleIPv4 map[string]bool
	singleIPv6 map[string]bool
	
	// Метаданные для диапазонов
	rangeMetadata map[string]*IPRangeMetadata
	
	// Кеш результатов
	cache     map[string]*IPCheckResult
	cacheTTL  time.Duration
	maxCache  int
	
	// Синхронизация
	mutex sync.RWMutex
	
	// Компоненты
	metrics *Metrics
	debug   *DebugConfig
	logger  *zap.Logger
	
	// Статистика
	totalChecks    int64
	botDetections  int64
	cacheHits      int64
	ipv4Checks     int64
	ipv6Checks     int64
	invalidIPs     int64
}

// IPRangeMetadata содержит метаданные о диапазоне IP
type IPRangeMetadata struct {
	Organization string
	Country      string
	BotType      BotType
	Description  string
	Source       string
	LastUpdated  time.Time
}

// IPCheckResult содержит результат проверки IP
type IPCheckResult struct {
	IsBot         bool
	MatchedRange  string
	Organization  string
	BotType       BotType
	Confidence    float64
	IPVersion     int
	Timestamp     time.Time
}

// NewIPRangeChecker создает новый экземпляр IPRangeChecker
func NewIPRangeChecker(config *Config, metrics *Metrics, debug *DebugConfig, logger *zap.Logger) *IPRangeChecker {
	irc := &IPRangeChecker{
		ipv4Networks:  make([]*net.IPNet, 0),
		ipv6Networks:  make([]*net.IPNet, 0),
		singleIPv4:    make(map[string]bool),
		singleIPv6:    make(map[string]bool),
		rangeMetadata: make(map[string]*IPRangeMetadata),
		cache:         make(map[string]*IPCheckResult),
		cacheTTL:      config.CacheTTL,
		maxCache:      5000, // Кеш для 5000 IP адресов
		metrics:       metrics,
		debug:         debug,
		logger:        logger,
	}

	// Используем кастомные диапазоны если заданы, иначе дефолтные
	ranges := config.BotIPRanges
	if len(ranges) == 0 {
		ranges = getDefaultBotIPRanges()
	}

	// Инициализация диапазонов
	if err := irc.initializeRanges(ranges); err != nil {
		logger.Error("failed to initialize IP ranges", zap.Error(err))
		return irc
	}

	// Загрузка метаданных по умолчанию
	irc.loadDefaultMetadata()

	logger.Info("IP range checker initialized",
		zap.Int("ipv4_networks", len(irc.ipv4Networks)),
		zap.Int("ipv6_networks", len(irc.ipv6Networks)),
		zap.Int("single_ipv4", len(irc.singleIPv4)),
		zap.Int("single_ipv6", len(irc.singleIPv6)),
		zap.Int("metadata_entries", len(irc.rangeMetadata)),
	)

	return irc
}

// initializeRanges инициализирует IP диапазоны из списка CIDR
func (irc *IPRangeChecker) initializeRanges(ranges []string) error {
	irc.mutex.Lock()
	defer irc.mutex.Unlock()

	// Очистка предыдущих данных
	irc.ipv4Networks = make([]*net.IPNet, 0, len(ranges))
	irc.ipv6Networks = make([]*net.IPNet, 0, len(ranges))
	irc.singleIPv4 = make(map[string]bool)
	irc.singleIPv6 = make(map[string]bool)

	for _, rangeStr := range ranges {
		if rangeStr == "" {
			continue
		}

		// Обработка одиночных IP адресов
		if !strings.Contains(rangeStr, "/") {
			ip := net.ParseIP(rangeStr)
			if ip == nil {
				irc.logger.Warn("invalid IP address", zap.String("ip", rangeStr))
				continue
			}

			if ip.To4() != nil {
				irc.singleIPv4[rangeStr] = true
			} else {
				irc.singleIPv6[rangeStr] = true
			}
			continue
		}

		// Обработка CIDR диапазонов
		_, ipNet, err := net.ParseCIDR(rangeStr)
		if err != nil {
			irc.logger.Warn("invalid CIDR range", 
				zap.String("range", rangeStr),
				zap.Error(err),
			)
			continue
		}

		// Определяем тип IP (IPv4 или IPv6)
		if ipNet.IP.To4() != nil {
			irc.ipv4Networks = append(irc.ipv4Networks, ipNet)
		} else {
			irc.ipv6Networks = append(irc.ipv6Networks, ipNet)
		}
	}

	// Сортировка сетей для оптимизации поиска
	irc.sortNetworks()

	return nil
}

// sortNetworks сортирует сети по размеру маски для оптимизации поиска
func (irc *IPRangeChecker) sortNetworks() {
	// Сортируем IPv4 сети (наименьшие маски первыми для более специфичных совпадений)
	sort.Slice(irc.ipv4Networks, func(i, j int) bool {
		ones1, _ := irc.ipv4Networks[i].Mask.Size()
		ones2, _ := irc.ipv4Networks[j].Mask.Size()
		return ones1 > ones2 // Более специфичные сети первыми
	})

	// Сортируем IPv6 сети
	sort.Slice(irc.ipv6Networks, func(i, j int) bool {
		ones1, _ := irc.ipv6Networks[i].Mask.Size()
		ones2, _ := irc.ipv6Networks[j].Mask.Size()
		return ones1 > ones2
	})
}

// IsBot проверяет, принадлежит ли IP адрес к диапазонам ботов
func (irc *IPRangeChecker) IsBot(ipStr string) (*IPCheckResult, error) {
	if ipStr == "" {
		return &IPCheckResult{
			IsBot:     false,
			IPVersion: 0,
			Timestamp: time.Now(),
		}, nil
	}

	// Извлекаем чистый IP (убираем порт если есть)
	cleanIP := irc.extractIP(ipStr)
	
	// Инкремент счетчика проверок
	irc.incrementTotalChecks()

	// Инкремент метрик для детального анализа
	if irc.metrics != nil {
		irc.metrics.IncrementIPRangeChecks()
	}

	// Проверка кеша
	if result := irc.getCachedResult(cleanIP); result != nil {
		irc.incrementCacheHits()
		if irc.metrics != nil {
			irc.metrics.IncrementCacheHits()
		}
		
		if irc.debug != nil {
			irc.debug.LogIPRangeCheck(cleanIP, result.IsBot, result.MatchedRange)
		}
		
		return result, nil
	}

	if irc.metrics != nil {
		irc.metrics.IncrementCacheMisses()
	}

	// Выполнение проверки
	result := irc.performCheck(cleanIP)
	
	// Сохранение в кеш
	irc.setCachedResult(cleanIP, result)
	
	// Логирование для дебага
	if irc.debug != nil {
		irc.debug.LogIPRangeCheck(cleanIP, result.IsBot, result.MatchedRange)
	}

	// Обновление статистики
	if result.IsBot {
		irc.incrementBotDetections()
	}

	return result, nil
}

// performCheck выполняет основную проверку IP адреса
func (irc *IPRangeChecker) performCheck(ipStr string) *IPCheckResult {
	irc.mutex.RLock()
	defer irc.mutex.RUnlock()

	// Парсинг IP адреса
	ip := net.ParseIP(ipStr)
	if ip == nil {
		irc.incrementInvalidIPs()
		return &IPCheckResult{
			IsBot:     false,
			IPVersion: 0,
			Timestamp: time.Now(),
		}
	}

	// Определение версии IP
	var ipVersion int
	var networks []*net.IPNet
	var singleIPs map[string]bool

	if ip.To4() != nil {
		ipVersion = 4
		networks = irc.ipv4Networks
		singleIPs = irc.singleIPv4
		irc.incrementIPv4Checks()
	} else {
		ipVersion = 6
		networks = irc.ipv6Networks
		singleIPs = irc.singleIPv6
		irc.incrementIPv6Checks()
	}

	// 1. Проверка отдельных IP адресов (самый быстрый)
	if singleIPs[ipStr] {
		metadata := irc.rangeMetadata[ipStr]
		return &IPCheckResult{
			IsBot:        true,
			MatchedRange: ipStr,
			Organization: irc.getOrganization(metadata),
			BotType:      irc.getBotType(metadata),
			Confidence:   1.0,
			IPVersion:    ipVersion,
			Timestamp:    time.Now(),
		}
	}

	// 2. Проверка CIDR диапазонов
	for _, network := range networks {
		if network.Contains(ip) {
			rangeStr := network.String()
			metadata := irc.rangeMetadata[rangeStr]
			
			return &IPCheckResult{
				IsBot:        true,
				MatchedRange: rangeStr,
				Organization: irc.getOrganization(metadata),
				BotType:      irc.getBotType(metadata),
				Confidence:   0.9,
				IPVersion:    ipVersion,
				Timestamp:    time.Now(),
			}
		}
	}

	// Не найдено совпадений
	return &IPCheckResult{
		IsBot:     false,
		IPVersion: ipVersion,
		Timestamp: time.Now(),
	}
}

// extractIP извлекает IP адрес из строки (убирает порт)
func (irc *IPRangeChecker) extractIP(address string) string {
	// Обработка IPv6 адресов с портом [::1]:8080
	if strings.HasPrefix(address, "[") {
		end := strings.Index(address, "]")
		if end != -1 {
			return address[1:end]
		}
	}

	// Обработка IPv4 адресов с портом 127.0.0.1:8080
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		// Если не удалось разделить, возвращаем как есть
		return address
	}
	return host
}

// getCachedResult получает результат из кеша
func (irc *IPRangeChecker) getCachedResult(ip string) *IPCheckResult {
	irc.mutex.RLock()
	defer irc.mutex.RUnlock()
	
	if result, exists := irc.cache[ip]; exists {
		// Проверка TTL
		if time.Since(result.Timestamp) < irc.cacheTTL {
			return result
		}
		// Удаление устаревшей записи
		delete(irc.cache, ip)
	}
	
	return nil
}

// setCachedResult сохраняет результат в кеш
func (irc *IPRangeChecker) setCachedResult(ip string, result *IPCheckResult) {
	irc.mutex.Lock()
	defer irc.mutex.Unlock()
	
	// Проверка размера кеша
	if len(irc.cache) >= irc.maxCache {
		irc.cleanupCache()
	}
	
	irc.cache[ip] = result
	
	if irc.debug != nil {
		irc.debug.LogCacheOperation(&CacheDebugInfo{
			Key:       ip,
			Operation: "set",
			Hit:       false,
			Value:     result,
			TTL:       irc.cacheTTL,
			Timestamp: time.Now(),
		})
	}
}

// cleanupCache очищает старые записи из кеша
func (irc *IPRangeChecker) cleanupCache() {
	now := time.Now()
	
	for key, result := range irc.cache {
		if now.Sub(result.Timestamp) > irc.cacheTTL {
			delete(irc.cache, key)
		}
	}
	
	// Если кеш все еще переполнен, удаляем самые старые записи
	if len(irc.cache) >= irc.maxCache {
		// Простая стратегия: удаляем половину записей
		count := 0
		target := len(irc.cache) / 2
		
		for key := range irc.cache {
			if count >= target {
				break
			}
			delete(irc.cache, key)
			count++
		}
	}
}

// AddRange добавляет новый IP диапазон в runtime
func (irc *IPRangeChecker) AddRange(rangeStr string, metadata *IPRangeMetadata) error {
	if rangeStr == "" {
		return fmt.Errorf("empty range string")
	}

	irc.mutex.Lock()
	defer irc.mutex.Unlock()

	// Обработка одиночного IP
	if !strings.Contains(rangeStr, "/") {
		ip := net.ParseIP(rangeStr)
		if ip == nil {
			return fmt.Errorf("invalid IP address: %s", rangeStr)
		}

		if ip.To4() != nil {
			irc.singleIPv4[rangeStr] = true
		} else {
			irc.singleIPv6[rangeStr] = true
		}
	} else {
		// Обработка CIDR диапазона
		_, ipNet, err := net.ParseCIDR(rangeStr)
		if err != nil {
			return fmt.Errorf("invalid CIDR range %s: %w", rangeStr, err)
		}

		if ipNet.IP.To4() != nil {
			irc.ipv4Networks = append(irc.ipv4Networks, ipNet)
		} else {
			irc.ipv6Networks = append(irc.ipv6Networks, ipNet)
		}

		// Пересортировка сетей
		irc.sortNetworks()
	}

	// Добавление метаданных
	if metadata != nil {
		irc.rangeMetadata[rangeStr] = metadata
	}

	// Очистка кеша после добавления нового диапазона
	irc.cache = make(map[string]*IPCheckResult)

	irc.logger.Info("added new IP range",
		zap.String("range", rangeStr),
		zap.String("organization", irc.getOrganization(metadata)),
	)

	return nil
}

// RemoveRange удаляет IP диапазон
func (irc *IPRangeChecker) RemoveRange(rangeStr string) error {
	irc.mutex.Lock()
	defer irc.mutex.Unlock()

	// Удаление одиночного IP
	if !strings.Contains(rangeStr, "/") {
		ip := net.ParseIP(rangeStr)
		if ip == nil {
			return fmt.Errorf("invalid IP address: %s", rangeStr)
		}

		if ip.To4() != nil {
			delete(irc.singleIPv4, rangeStr)
		} else {
			delete(irc.singleIPv6, rangeStr)
		}
	} else {
		// Удаление CIDR диапазона
		_, targetNet, err := net.ParseCIDR(rangeStr)
		if err != nil {
			return fmt.Errorf("invalid CIDR range %s: %w", rangeStr, err)
		}

		if targetNet.IP.To4() != nil {
			// Удаление из IPv4 сетей
			for i, network := range irc.ipv4Networks {
				if network.String() == rangeStr {
					irc.ipv4Networks = append(irc.ipv4Networks[:i], irc.ipv4Networks[i+1:]...)
					break
				}
			}
		} else {
			// Удаление из IPv6 сетей
			for i, network := range irc.ipv6Networks {
				if network.String() == rangeStr {
					irc.ipv6Networks = append(irc.ipv6Networks[:i], irc.ipv6Networks[i+1:]...)
					break
				}
			}
		}
	}

	// Удаление метаданных
	delete(irc.rangeMetadata, rangeStr)

	// Очистка кеша
	irc.cache = make(map[string]*IPCheckResult)

	irc.logger.Info("removed IP range",
		zap.String("range", rangeStr),
	)

	return nil
}

// loadDefaultMetadata загружает метаданные по умолчанию для известных диапазонов
func (irc *IPRangeChecker) loadDefaultMetadata() {
	defaultMetadata := map[string]*IPRangeMetadata{
		// Google
		"66.249.64.0/19": {
			Organization: "Google LLC",
			Country:      "US",
			BotType:      BotTypeSearch,
			Description:  "Googlebot crawler",
			Source:       "Google",
			LastUpdated:  time.Now(),
		},
		"64.233.160.0/19": {
			Organization: "Google LLC",
			Country:      "US", 
			BotType:      BotTypeSearch,
			Description:  "Google services",
			Source:       "Google",
			LastUpdated:  time.Now(),
		},
		"72.14.192.0/18": {
			Organization: "Google LLC",
			Country:      "US",
			BotType:      BotTypeSearch,
			Description:  "Google infrastructure",
			Source:       "Google",
			LastUpdated:  time.Now(),
		},

		// Microsoft Bing
		"40.77.167.0/24": {
			Organization: "Microsoft Corporation",
			Country:      "US",
			BotType:      BotTypeSearch,
			Description:  "Bingbot crawler",
			Source:       "Microsoft",
			LastUpdated:  time.Now(),
		},
		"157.55.39.0/24": {
			Organization: "Microsoft Corporation",
			Country:      "US",
			BotType:      BotTypeSearch,
			Description:  "Bingbot services",
			Source:       "Microsoft",
			LastUpdated:  time.Now(),
		},

		// Yandex
		"5.45.192.0/18": {
			Organization: "Yandex LLC",
			Country:      "RU",
			BotType:      BotTypeSearch,
			Description:  "YandexBot crawler",
			Source:       "Yandex",
			LastUpdated:  time.Now(),
		},
		"95.108.128.0/17": {
			Organization: "Yandex LLC",
			Country:      "RU",
			BotType:      BotTypeSearch,
			Description:  "Yandex services",
			Source:       "Yandex",
			LastUpdated:  time.Now(),
		},

		// Facebook
		"31.13.24.0/21": {
			Organization: "Meta Platforms Inc",
			Country:      "US",
			BotType:      BotTypeSocial,
			Description:  "Facebook crawler",
			Source:       "Facebook",
			LastUpdated:  time.Now(),
		},
		"173.252.64.0/18": {
			Organization: "Meta Platforms Inc",
			Country:      "US",
			BotType:      BotTypeSocial,
			Description:  "Facebook infrastructure",
			Source:       "Facebook",
			LastUpdated:  time.Now(),
		},
	}

	irc.mutex.Lock()
	for rangeStr, metadata := range defaultMetadata {
		irc.rangeMetadata[rangeStr] = metadata
	}
	irc.mutex.Unlock()
}

// Вспомогательные методы

func (irc *IPRangeChecker) getOrganization(metadata *IPRangeMetadata) string {
	if metadata != nil && metadata.Organization != "" {
		return metadata.Organization
	}
	return "Unknown"
}

func (irc *IPRangeChecker) getBotType(metadata *IPRangeMetadata) BotType {
	if metadata != nil {
		return metadata.BotType
	}
	return BotTypeUnknown
}

// GetStats возвращает статистику
func (irc *IPRangeChecker) GetStats() map[string]interface{} {
	irc.mutex.RLock()
	defer irc.mutex.RUnlock()
	
	hitRate := 0.0
	if irc.totalChecks > 0 {
		hitRate = float64(irc.cacheHits) / float64(irc.totalChecks)
	}
	
	detectionRate := 0.0
	if irc.totalChecks > 0 {
		detectionRate = float64(irc.botDetections) / float64(irc.totalChecks)
	}
	
	return map[string]interface{}{
		"ipv4_networks":    len(irc.ipv4Networks),
		"ipv6_networks":    len(irc.ipv6Networks),
		"single_ipv4":      len(irc.singleIPv4),
		"single_ipv6":      len(irc.singleIPv6),
		"cache_size":       len(irc.cache),
		"cache_max_size":   irc.maxCache,
		"total_checks":     irc.totalChecks,
		"bot_detections":   irc.botDetections,
		"cache_hits":       irc.cacheHits,
		"cache_hit_rate":   hitRate,
		"detection_rate":   detectionRate,
		"ipv4_checks":      irc.ipv4Checks,
		"ipv6_checks":      irc.ipv6Checks,
		"invalid_ips":      irc.invalidIPs,
		"metadata_entries": len(irc.rangeMetadata),
	}
}

// ClearCache очищает кеш
func (irc *IPRangeChecker) ClearCache() {
	irc.mutex.Lock()
	defer irc.mutex.Unlock()
	
	irc.cache = make(map[string]*IPCheckResult)
	irc.logger.Info("IP range checker cache cleared")
}

// Методы для статистики
func (irc *IPRangeChecker) incrementTotalChecks() {
	irc.mutex.Lock()
	defer irc.mutex.Unlock()
	irc.totalChecks++
}

func (irc *IPRangeChecker) incrementBotDetections() {
	irc.mutex.Lock()
	defer irc.mutex.Unlock()
	irc.botDetections++
}

func (irc *IPRangeChecker) incrementCacheHits() {
	irc.mutex.Lock()
	defer irc.mutex.Unlock()
	irc.cacheHits++
}

func (irc *IPRangeChecker) incrementIPv4Checks() {
	irc.mutex.Lock()
	defer irc.mutex.Unlock()
	irc.ipv4Checks++
}

func (irc *IPRangeChecker) incrementIPv6Checks() {
	irc.mutex.Lock()
	defer irc.mutex.Unlock()
	irc.ipv6Checks++
}

func (irc *IPRangeChecker) incrementInvalidIPs() {
	irc.mutex.Lock()
	defer irc.mutex.Unlock()
	irc.invalidIPs++
}