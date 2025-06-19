package botredirect

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// DebugConfig содержит конфигурацию для дебаг-режима
type DebugConfig struct {
	Enabled        bool
	LogAllRequests bool
	LogDNSQueries  bool
	LogCacheOps    bool
	VerboseMetrics bool
	logger         *zap.Logger
}

// RequestDebugInfo содержит отладочную информацию о запросе
type RequestDebugInfo struct {
	IP           string
	UserAgent    string
	Referer      string
	Method       string
	URL          string
	Headers      map[string]string
	StartTime    time.Time
	ProcessingSteps []ProcessingStep
}

// ProcessingStep представляет один шаг обработки запроса
type ProcessingStep struct {
	Step      string
	Result    string
	Duration  time.Duration
	Details   map[string]interface{}
	Timestamp time.Time
}

// DNSDebugInfo содержит отладочную информацию о DNS запросах
type DNSDebugInfo struct {
	IP           string
	Hostname     string
	QueryType    string
	Result       string
	Duration     time.Duration
	Error        string
	Timestamp    time.Time
}

// CacheDebugInfo содержит отладочную информацию о операциях с кешем
type CacheDebugInfo struct {
	Key       string
	Operation string
	Hit       bool
	Value     interface{}
	TTL       time.Duration
	Timestamp time.Time
}

// NewDebugConfig создает новую конфигурацию дебаг-режима
func NewDebugConfig(config *Config, logger *zap.Logger) *DebugConfig {
	return &DebugConfig{
		Enabled:        config.EnableDebug,
		LogAllRequests: config.LogAllRequests,
		LogDNSQueries:  config.LogDNSQueries,
		LogCacheOps:    config.LogCacheOps,
		VerboseMetrics: config.VerboseMetrics,
		logger:         logger,
	}
}

// StartRequestDebug начинает отладку запроса
func (dc *DebugConfig) StartRequestDebug(r *http.Request) *RequestDebugInfo {
	if !dc.Enabled || !dc.LogAllRequests {
		return nil
	}

	info := &RequestDebugInfo{
		IP:        r.RemoteAddr,
		UserAgent: r.UserAgent(),
		Referer:   r.Referer(),
		Method:    r.Method,
		URL:       r.URL.String(),
		Headers:   make(map[string]string),
		StartTime: time.Now(),
		ProcessingSteps: make([]ProcessingStep, 0),
	}

	// Копируем важные заголовки
	importantHeaders := []string{
		"X-Forwarded-For",
		"X-Real-IP",
		"CF-Connecting-IP",
		"Accept",
		"Accept-Language",
		"Accept-Encoding",
		"Connection",
		"Upgrade-Insecure-Requests",
		"Sec-Fetch-Dest",
		"Sec-Fetch-Mode",
		"Sec-Fetch-Site",
	}

	for _, header := range importantHeaders {
		if value := r.Header.Get(header); value != "" {
			info.Headers[header] = value
		}
	}

	dc.logger.Debug("started request debug",
		zap.String("ip", info.IP),
		zap.String("user_agent", info.UserAgent),
		zap.String("referer", info.Referer),
		zap.String("url", info.URL),
	)

	return info
}

// AddProcessingStep добавляет шаг обработки в отладочную информацию
func (dc *DebugConfig) AddProcessingStep(info *RequestDebugInfo, step, result string, duration time.Duration, details map[string]interface{}) {
	if !dc.Enabled || info == nil {
		return
	}

	processingStep := ProcessingStep{
		Step:      step,
		Result:    result,
		Duration:  duration,
		Details:   details,
		Timestamp: time.Now(),
	}

	info.ProcessingSteps = append(info.ProcessingSteps, processingStep)

	dc.logger.Debug("processing step completed",
		zap.String("ip", info.IP),
		zap.String("step", step),
		zap.String("result", result),
		zap.Duration("duration", duration),
		zap.Any("details", details),
	)
}

// FinishRequestDebug завершает отладку запроса
func (dc *DebugConfig) FinishRequestDebug(info *RequestDebugInfo, finalResult string) {
	if !dc.Enabled || info == nil {
		return
	}

	totalDuration := time.Since(info.StartTime)

	// Логируем общую информацию о запросе
	dc.logger.Info("request processing completed",
		zap.String("ip", info.IP),
		zap.String("user_agent", info.UserAgent),
		zap.String("referer", info.Referer),
		zap.String("final_result", finalResult),
		zap.Duration("total_duration", totalDuration),
		zap.Int("processing_steps", len(info.ProcessingSteps)),
	)

	// Детальная информация о каждом шаге
	for i, step := range info.ProcessingSteps {
		dc.logger.Debug("processing step detail",
			zap.String("ip", info.IP),
			zap.Int("step_number", i+1),
			zap.String("step_name", step.Step),
			zap.String("step_result", step.Result),
			zap.Duration("step_duration", step.Duration),
			zap.Any("step_details", step.Details),
		)
	}
}

// LogDNSQuery логирует DNS запрос
func (dc *DebugConfig) LogDNSQuery(info *DNSDebugInfo) {
	if !dc.Enabled || !dc.LogDNSQueries {
		return
	}

	if info.Error != "" {
		dc.logger.Warn("DNS query failed",
			zap.String("ip", info.IP),
			zap.String("hostname", info.Hostname),
			zap.String("query_type", info.QueryType),
			zap.String("error", info.Error),
			zap.Duration("duration", info.Duration),
		)
	} else {
		dc.logger.Debug("DNS query completed",
			zap.String("ip", info.IP),
			zap.String("hostname", info.Hostname),
			zap.String("query_type", info.QueryType),
			zap.String("result", info.Result),
			zap.Duration("duration", info.Duration),
		)
	}
}

// LogCacheOperation логирует операцию с кешем
func (dc *DebugConfig) LogCacheOperation(info *CacheDebugInfo) {
	if !dc.Enabled || !dc.LogCacheOps {
		return
	}

	dc.logger.Debug("cache operation",
		zap.String("key", info.Key),
		zap.String("operation", info.Operation),
		zap.Bool("hit", info.Hit),
		zap.Duration("ttl", info.TTL),
		zap.Time("timestamp", info.Timestamp),
	)
}

// LogUserAgentCheck логирует проверку User-Agent
func (dc *DebugConfig) LogUserAgentCheck(userAgent string, isBot bool, matchedPattern string) {
	if !dc.Enabled {
		return
	}

	dc.logger.Debug("user agent check",
		zap.String("user_agent", userAgent),
		zap.Bool("is_bot", isBot),
		zap.String("matched_pattern", matchedPattern),
	)
}

// LogIPRangeCheck логирует проверку IP диапазона
func (dc *DebugConfig) LogIPRangeCheck(ip string, isBot bool, matchedRange string) {
	if !dc.Enabled {
		return
	}

	dc.logger.Debug("IP range check",
		zap.String("ip", ip),
		zap.Bool("is_bot", isBot),
		zap.String("matched_range", matchedRange),
	)
}

// LogReferrerCheck логирует проверку referrer
func (dc *DebugConfig) LogReferrerCheck(referer string, isFromSearch bool, matchedDomain string) {
	if !dc.Enabled {
		return
	}

	dc.logger.Debug("referrer check",
		zap.String("referer", referer),
		zap.Bool("is_from_search", isFromSearch),
		zap.String("matched_domain", matchedDomain),
	)
}

// LogReverseDNSCheck логирует проверку обратного DNS
func (dc *DebugConfig) LogReverseDNSCheck(ip string, hostname string, isValid bool, verifiedIP string) {
	if !dc.Enabled {
		return
	}

	dc.logger.Debug("reverse DNS check",
		zap.String("ip", ip),
		zap.String("hostname", hostname),
		zap.Bool("is_valid", isValid),
		zap.String("verified_ip", verifiedIP),
	)
}

// LogCacheStats логирует статистику кеша
func (dc *DebugConfig) LogCacheStats(size int, hits int64, misses int64, hitRate float64) {
	if !dc.Enabled || !dc.VerboseMetrics {
		return
	}

	dc.logger.Info("cache statistics",
		zap.Int("size", size),
		zap.Int64("hits", hits),
		zap.Int64("misses", misses),
		zap.Float64("hit_rate", hitRate),
	)
}

// LogRateLimitEvent логирует событие rate limiting
func (dc *DebugConfig) LogRateLimitEvent(ip string, limitType string, allowed bool, currentRate int, maxRate int) {
	if !dc.Enabled {
		return
	}

	if allowed {
		dc.logger.Debug("rate limit check passed",
			zap.String("ip", ip),
			zap.String("limit_type", limitType),
			zap.Int("current_rate", currentRate),
			zap.Int("max_rate", maxRate),
		)
	} else {
		dc.logger.Warn("rate limit exceeded",
			zap.String("ip", ip),
			zap.String("limit_type", limitType),
			zap.Int("current_rate", currentRate),
			zap.Int("max_rate", maxRate),
		)
	}
}

// IsEnabled возвращает статус включенности дебаг-режима
func (dc *DebugConfig) IsEnabled() bool {
	return dc.Enabled
}

// SetLogLevel динамически изменяет уровень логирования
func (dc *DebugConfig) SetLogLevel(level string) {
	// В реальной реализации здесь должно быть изменение уровня логирования
	dc.logger.Info("debug log level changed",
		zap.String("new_level", level),
	)
}

// EnableVerboseLogging включает детальное логирование
func (dc *DebugConfig) EnableVerboseLogging() {
	dc.LogAllRequests = true
	dc.LogDNSQueries = true
	dc.LogCacheOps = true
	dc.VerboseMetrics = true
	
	dc.logger.Info("verbose logging enabled")
}

// DisableVerboseLogging отключает детальное логирование
func (dc *DebugConfig) DisableVerboseLogging() {
	dc.LogAllRequests = false
	dc.LogDNSQueries = false
	dc.LogCacheOps = false
	dc.VerboseMetrics = false
	
	dc.logger.Info("verbose logging disabled")
}