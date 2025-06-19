package botredirect

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// BotDetector главный компонент для определения ботов
type BotDetector struct {
	// Основные компоненты проверки
	userAgentMatcher  *UserAgentMatcher
	ipRangeChecker    *IPRangeChecker
	reverseDNSChecker *ReverseDNSChecker
	referrerChecker   *ReferrerChecker

	// Системные компоненты
	cache       *Cache
	templates   *Templates
	metrics     *Metrics
	rateLimiter *RateLimiter
	debug       *DebugConfig

	// Конфигурация
	config *Config
	logger *zap.Logger

	// Статистика (используем atomic для thread-safety)
	totalChecks    int64
	botDetections  int64
	userRedirects  int64
	directUsers    int64
	checkDurations []time.Duration
	mutex          sync.RWMutex
}

// DetectionResult результат проверки на бота
type DetectionResult struct {
	IsBot           bool
	UserType        UserType
	DetectionMethod string
	Confidence      float64
	MatchedPattern  string
	ProcessingTime  time.Duration
	Details         map[string]interface{}
	Timestamp       time.Time
}

// NewBotDetector создает новый экземпляр детектора ботов
func NewBotDetector(config *Config, logger *zap.Logger) *BotDetector {
	bd := &BotDetector{
		config:         config,
		logger:         logger,
		checkDurations: make([]time.Duration, 0, 1000),
	}

	// Инициализируем компоненты в правильном порядке

	// 1. Метрики (должны быть первыми)
	bd.metrics = NewMetrics(config.EnableMetrics, config.VerboseMetrics, logger)

	// 2. Debug конфигурация
	bd.debug = NewDebugConfig(config, logger)

	// 3. Cache система
	bd.cache = NewCache(config, bd.metrics, bd.debug, logger)

	// 4. Rate Limiter
	bd.rateLimiter = NewRateLimiter(config, bd.metrics, logger)

	// 5. Templates система
	bd.templates = NewTemplates(config, logger)

	// 6. Компоненты проверки
	bd.userAgentMatcher = NewUserAgentMatcher(config, bd.metrics, bd.debug, logger)
	bd.ipRangeChecker = NewIPRangeChecker(config, bd.metrics, bd.debug, logger)
	bd.reverseDNSChecker = NewReverseDNSChecker(config, bd.metrics, bd.debug, logger)
	bd.referrerChecker = NewReferrerChecker(config, bd.metrics, bd.debug, logger)

	logger.Info("bot detector initialized",
		zap.Bool("user_agent_enabled", bd.userAgentMatcher != nil),
		zap.Bool("ip_range_enabled", bd.ipRangeChecker != nil),
		zap.Bool("reverse_dns_enabled", bd.reverseDNSChecker != nil && config.EnableReverseDNS),
		zap.Bool("referrer_enabled", bd.referrerChecker != nil && config.EnableReferrerCheck),
		zap.Bool("cache_enabled", bd.cache != nil),
		zap.Bool("metrics_enabled", bd.metrics != nil),
	)

	return bd
}

// DetectBot выполняет полную проверку на бота
func (bd *BotDetector) DetectBot(r *http.Request) *DetectionResult {
	startTime := time.Now()

	// Инкремент общих проверок
	atomic.AddInt64(&bd.totalChecks, 1)

	// Извлекаем данные запроса
	clientIP := r.RemoteAddr
	userAgent := r.UserAgent()

	// Начинаем отладку если включена
	var debugInfo *RequestDebugInfo
	if bd.debug != nil {
		debugInfo = bd.debug.StartRequestDebug(r)
	}

	// Проверяем кеш для быстрого ответа
	cacheKey := bd.generateCacheKey(clientIP, userAgent)
	if cached := bd.cache.Get(cacheKey); cached != nil {
		if result, ok := cached.(*DetectionResult); ok {
			// Проверяем актуальность кешированного результата
			if time.Since(result.Timestamp) < bd.config.CacheTTL {
				result.ProcessingTime = time.Since(startTime)

				if bd.debug != nil && debugInfo != nil {
					bd.debug.AddProcessingStep(debugInfo, "cache_hit", result.UserType.String(),
						result.ProcessingTime, map[string]interface{}{
							"detection_method": result.DetectionMethod,
							"confidence":       result.Confidence,
						})
					bd.debug.FinishRequestDebug(debugInfo, "cache_result")
				}

				return result
			}
		}
	}

	// Выполняем проверки по приоритету
	result := bd.performDetection(r, debugInfo)
	result.ProcessingTime = time.Since(startTime)
	result.Timestamp = time.Now()

	// Сохраняем в кеш
	bd.cache.SetWithTTL(cacheKey, result, bd.config.CacheTTL)

	// Обновляем статистику
	bd.updateStatistics(result)

	// Завершаем отладку
	if bd.debug != nil && debugInfo != nil {
		bd.debug.FinishRequestDebug(debugInfo, result.UserType.String())
	}

	return result
}

// performDetection выполняет основную логику детекции
func (bd *BotDetector) performDetection(r *http.Request, debugInfo *RequestDebugInfo) *DetectionResult {
	clientIP := r.RemoteAddr
	userAgent := r.UserAgent()

	// 1. Проверка User-Agent (быстрая, высокая точность)
	if bd.userAgentMatcher != nil {
		stepStart := time.Now()
		uaResult, err := bd.userAgentMatcher.IsBot(userAgent)

		// ИСПРАВЛЕНИЕ: Корректная обработка ошибок
		if err != nil {
			bd.logger.Warn("user agent check failed",
				zap.String("user_agent", userAgent),
				zap.Error(err),
			)
			// Продолжаем с другими проверками
		} else if uaResult.IsBot {
			stepDuration := time.Since(stepStart)

			if bd.debug != nil && debugInfo != nil {
				bd.debug.AddProcessingStep(debugInfo, "user_agent_check", "bot_detected",
					stepDuration, map[string]interface{}{
						"matched_pattern": uaResult.MatchedPattern,
						"bot_type":        uaResult.BotType,
						"confidence":      uaResult.Confidence,
					})
			}

			return &DetectionResult{
				IsBot:           true,
				UserType:        UserTypeBot,
				DetectionMethod: "user_agent",
				Confidence:      uaResult.Confidence,
				MatchedPattern:  uaResult.MatchedPattern,
				Details: map[string]interface{}{
					"bot_type":   uaResult.BotType,
					"user_agent": userAgent,
				},
			}
		}

		if bd.debug != nil && debugInfo != nil && err == nil {
			bd.debug.AddProcessingStep(debugInfo, "user_agent_check", "no_match",
				time.Since(stepStart), map[string]interface{}{
					"user_agent": userAgent,
				})
		}
	}

	// 2. Проверка IP диапазонов (быстрая, высокая точность)
	if bd.ipRangeChecker != nil {
		stepStart := time.Now()
		ipResult, err := bd.ipRangeChecker.IsBot(clientIP)

		if err != nil {
			bd.logger.Warn("IP range check failed",
				zap.String("client_ip", clientIP),
				zap.Error(err),
			)
		} else if ipResult.IsBot {
			stepDuration := time.Since(stepStart)

			if bd.debug != nil && debugInfo != nil {
				bd.debug.AddProcessingStep(debugInfo, "ip_range_check", "bot_detected",
					stepDuration, map[string]interface{}{
						"matched_range": ipResult.MatchedRange,
						"organization":  ipResult.Organization,
						"confidence":    ipResult.Confidence,
					})
			}

			return &DetectionResult{
				IsBot:           true,
				UserType:        UserTypeBot,
				DetectionMethod: "ip_range",
				Confidence:      ipResult.Confidence,
				MatchedPattern:  ipResult.MatchedRange,
				Details: map[string]interface{}{
					"organization": ipResult.Organization,
					"bot_type":     ipResult.BotType,
					"ip_version":   ipResult.IPVersion,
				},
			}
		}

		if bd.debug != nil && debugInfo != nil && err == nil {
			bd.debug.AddProcessingStep(debugInfo, "ip_range_check", "no_match",
				time.Since(stepStart), map[string]interface{}{
					"client_ip": clientIP,
				})
		}
	}

	// 3. Обратный DNS (медленная, но очень точная проверка)
	if bd.reverseDNSChecker != nil && bd.config.EnableReverseDNS {
		stepStart := time.Now()

		// Сначала проверяем кеш DNS
		dnsResult, err := bd.reverseDNSChecker.CheckDNS(clientIP)
		if err != nil {
			bd.logger.Warn("reverse DNS check failed",
				zap.String("client_ip", clientIP),
				zap.Error(err),
			)
		} else if dnsResult.IsBot {
			stepDuration := time.Since(stepStart)

			if bd.debug != nil && debugInfo != nil {
				bd.debug.AddProcessingStep(debugInfo, "reverse_dns_check", "bot_detected",
					stepDuration, map[string]interface{}{
						"hostname":    dnsResult.Hostname,
						"verified_ip": dnsResult.VerifiedIP,
						"confidence":  dnsResult.Confidence,
					})
			}

			return &DetectionResult{
				IsBot:           true,
				UserType:        UserTypeBot,
				DetectionMethod: "reverse_dns",
				Confidence:      dnsResult.Confidence,
				MatchedPattern:  dnsResult.Hostname,
				Details: map[string]interface{}{
					"hostname":     dnsResult.Hostname,
					"verified_ip":  dnsResult.VerifiedIP,
					"organization": dnsResult.Organization,
					"bot_type":     dnsResult.BotType,
				},
			}
		}

		if bd.debug != nil && debugInfo != nil && err == nil {
			bd.debug.AddProcessingStep(debugInfo, "reverse_dns_check", "no_match",
				time.Since(stepStart), map[string]interface{}{
					"client_ip": clientIP,
				})
		}
	}

	// 4. Определение типа обычного пользователя через Referrer
	return bd.determineUserType(r, debugInfo)
}

// determineUserType определяет тип обычного пользователя
func (bd *BotDetector) determineUserType(r *http.Request, debugInfo *RequestDebugInfo) *DetectionResult {
	referer := r.Referer()

	if bd.referrerChecker != nil && bd.config.EnableReferrerCheck {
		stepStart := time.Now()

		refResult, err := bd.referrerChecker.CheckReferrer(referer)
		if err != nil {
			bd.logger.Warn("referrer check failed",
				zap.String("referer", referer),
				zap.Error(err),
			)
		} else {
			stepDuration := time.Since(stepStart)

			if refResult.IsFromSearch {
				if bd.debug != nil && debugInfo != nil {
					bd.debug.AddProcessingStep(debugInfo, "referrer_check", "search_engine",
						stepDuration, map[string]interface{}{
							"search_engine": refResult.SearchEngine,
							"domain":        refResult.Domain,
							"confidence":    refResult.Confidence,
						})
				}

				return &DetectionResult{
					IsBot:           false,
					UserType:        UserTypeFromSearch,
					DetectionMethod: "referrer",
					Confidence:      refResult.Confidence,
					MatchedPattern:  refResult.MatchedPattern,
					Details: map[string]interface{}{
						"search_engine":    refResult.SearchEngine,
						"domain":           refResult.Domain,
						"referrer_type":    refResult.ReferrerType,
						"query_parameters": refResult.QueryParameters,
					},
				}
			} else {
				if bd.debug != nil && debugInfo != nil {
					bd.debug.AddProcessingStep(debugInfo, "referrer_check", "direct_user",
						stepDuration, map[string]interface{}{
							"referrer_type": refResult.ReferrerType,
							"domain":        refResult.Domain,
						})
				}

				return &DetectionResult{
					IsBot:           false,
					UserType:        UserTypeDirect,
					DetectionMethod: "referrer",
					Confidence:      refResult.Confidence,
					MatchedPattern:  "",
					Details: map[string]interface{}{
						"referrer_type": refResult.ReferrerType,
						"domain":        refResult.Domain,
					},
				}
			}
		}
	}

	// Fallback логика без referrer checker
	if referer == "" {
		return &DetectionResult{
			IsBot:           false,
			UserType:        UserTypeDirect,
			DetectionMethod: "fallback",
			Confidence:      0.8,
			MatchedPattern:  "",
			Details: map[string]interface{}{
				"reason": "empty_referrer",
			},
		}
	}

	// По умолчанию считаем пользователем с поисковика
	return &DetectionResult{
		IsBot:           false,
		UserType:        UserTypeFromSearch,
		DetectionMethod: "fallback",
		Confidence:      0.6,
		MatchedPattern:  "",
		Details: map[string]interface{}{
			"reason":   "default_search_user",
			"referrer": referer,
		},
	}
}

// generateCacheKey генерирует ключ для кеширования
func (bd *BotDetector) generateCacheKey(ip, userAgent string) string {
	// Используем комбинацию IP и User-Agent для ключа
	return ip + "|" + userAgent
}

// updateStatistics обновляет внутреннюю статистику
func (bd *BotDetector) updateStatistics(result *DetectionResult) {
	// Обновляем счетчики
	switch result.UserType {
	case UserTypeBot:
		atomic.AddInt64(&bd.botDetections, 1)
		if bd.metrics != nil {
			bd.metrics.IncrementBotRequests()
		}
	case UserTypeFromSearch:
		atomic.AddInt64(&bd.userRedirects, 1)
		if bd.metrics != nil {
			bd.metrics.IncrementSearchUserRequests()
		}
	case UserTypeDirect:
		atomic.AddInt64(&bd.directUsers, 1)
		if bd.metrics != nil {
			bd.metrics.IncrementDirectUserRequests()
		}
	}

	// Записываем время обработки
	bd.mutex.Lock()
	if len(bd.checkDurations) >= 1000 {
		// Сдвигаем слайс вместо переаллокации
		copy(bd.checkDurations, bd.checkDurations[1:])
		bd.checkDurations = bd.checkDurations[:len(bd.checkDurations)-1]
	}
	bd.checkDurations = append(bd.checkDurations, result.ProcessingTime)
	bd.mutex.Unlock()

	// Обновляем метрики
	if bd.metrics != nil {
		bd.metrics.RecordProcessingTime(result.ProcessingTime)
	}
}

// GetTemplates возвращает систему шаблонов
func (bd *BotDetector) GetTemplates() *Templates {
	return bd.templates
}

// GetMetrics возвращает систему метрик
func (bd *BotDetector) GetMetrics() *Metrics {
	return bd.metrics
}

// GetRateLimiter возвращает rate limiter
func (bd *BotDetector) GetRateLimiter() *RateLimiter {
	return bd.rateLimiter
}

// GetStats возвращает статистику детектора
func (bd *BotDetector) GetStats() map[string]interface{} {
	bd.mutex.RLock()
	checkDurationsLen := len(bd.checkDurations)
	var avgProcessingTime time.Duration
	if checkDurationsLen > 0 {
		var total time.Duration
		for _, d := range bd.checkDurations {
			total += d
		}
		avgProcessingTime = total / time.Duration(checkDurationsLen)
	}
	bd.mutex.RUnlock()

	totalChecks := atomic.LoadInt64(&bd.totalChecks)
	botDetections := atomic.LoadInt64(&bd.botDetections)
	userRedirects := atomic.LoadInt64(&bd.userRedirects)
	directUsers := atomic.LoadInt64(&bd.directUsers)

	stats := map[string]interface{}{
		"total_checks":        totalChecks,
		"bot_detections":      botDetections,
		"user_redirects":      userRedirects,
		"direct_users":        directUsers,
		"avg_processing_time": avgProcessingTime,
		"components_enabled": map[string]bool{
			"user_agent_matcher":  bd.userAgentMatcher != nil,
			"ip_range_checker":    bd.ipRangeChecker != nil,
			"reverse_dns_checker": bd.reverseDNSChecker != nil && bd.config.EnableReverseDNS,
			"referrer_checker":    bd.referrerChecker != nil && bd.config.EnableReferrerCheck,
			"cache":               bd.cache != nil,
			"metrics":             bd.metrics != nil,
			"rate_limiter":        bd.rateLimiter != nil,
		},
	}

	// Добавляем статистику компонентов
	if bd.userAgentMatcher != nil {
		stats["user_agent_stats"] = bd.userAgentMatcher.GetStats()
	}

	if bd.ipRangeChecker != nil {
		stats["ip_range_stats"] = bd.ipRangeChecker.GetStats()
	}

	if bd.reverseDNSChecker != nil {
		stats["reverse_dns_stats"] = bd.reverseDNSChecker.GetStats()
	}

	if bd.referrerChecker != nil {
		stats["referrer_stats"] = bd.referrerChecker.GetStats()
	}

	if bd.cache != nil {
		stats["cache_stats"] = bd.cache.GetStats()
	}

	return stats
}

// Shutdown gracefully останавливает все компоненты
func (bd *BotDetector) Shutdown() {
	bd.logger.Info("shutting down bot detector")

	// Останавливаем компоненты в обратном порядке
	if bd.reverseDNSChecker != nil {
		bd.reverseDNSChecker.Shutdown()
	}

	if bd.rateLimiter != nil {
		bd.rateLimiter.Shutdown()
	}

	if bd.cache != nil {
		bd.cache.StopCleanup()
	}

	if bd.metrics != nil && bd.metrics.enabled {
		bd.metrics.LogStats()
	}

	bd.logger.Info("bot detector shutdown completed")
}
