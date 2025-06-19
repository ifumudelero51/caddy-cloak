package botredirect

import (
	"time"
)

// Config содержит всю конфигурацию плагина
type Config struct {
	// URL для перенаправления пользователей с поисковиков
	RedirectURL string `json:"redirect_url"`

	// Список CIDR диапазонов IP-адресов ботов
	BotIPRanges []string `json:"bot_ip_ranges"`

	// Список паттернов User-Agent для ботов
	BotUserAgents []string `json:"bot_user_agents"`

	// Список разрешенных referrer доменов (поисковые системы)
	AllowedReferrers []string `json:"allowed_referrers"`

	// Включить проверку обратного DNS
	EnableReverseDNS bool `json:"enable_reverse_dns"`

	// Включить проверку HTTP Referer заголовка
	EnableReferrerCheck bool `json:"enable_referrer_check"`

	// Включить систему метрик
	EnableMetrics bool `json:"enable_metrics"`

	// Включить rate limiting для защиты от DoS
	EnableRateLimit bool `json:"enable_rate_limit"`

	// Включить дебаг-режим
	EnableDebug bool `json:"enable_debug"`

	// HTML шаблон для пустой страницы
	EmptyPageTemplate string `json:"empty_page_template"`

	// Время жизни кеша
	CacheTTL time.Duration `json:"cache_ttl"`

	// Таймаут для DNS запросов
	DNSTimeout time.Duration `json:"dns_timeout"`

	// Максимальное количество DNS запросов в секунду на IP
	MaxDNSPerSecond int `json:"max_dns_per_second"`

	// Максимальное количество запросов в секунду на IP
	MaxRequestsPerIP int `json:"max_requests_per_ip"`

	// Окно для подсчета rate limit (в секундах)
	RateLimitWindow time.Duration `json:"rate_limit_window"`

	// Максимальный размер кеша
	MaxCacheSize int `json:"max_cache_size"`

	// Интервал очистки кеша
	CleanupInterval time.Duration `json:"cleanup_interval"`

	// Размер пула для DNS worker'ов
	DNSWorkerPoolSize int `json:"dns_worker_pool_size"`

	// Размер буфера для DNS очереди
	DNSQueueSize int `json:"dns_queue_size"`

	// Уровень логирования
	LogLevel string `json:"log_level"`

	// Логировать все запросы (для дебага)
	LogAllRequests bool `json:"log_all_requests"`

	// Логировать DNS запросы (для дебага)
	LogDNSQueries bool `json:"log_dns_queries"`

	// Логировать операции с кешем (для дебага)
	LogCacheOps bool `json:"log_cache_ops"`

	// Детальные метрики (для дебага)
	VerboseMetrics bool `json:"verbose_metrics"`

	// Путь для экспорта метрик (опционально)
	MetricsPath string `json:"metrics_path"`

	// Включить Prometheus метрики
	EnablePrometheus bool `json:"enable_prometheus"`
}

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() *Config {
	return &Config{
		RedirectURL:         "",
		BotIPRanges:         getDefaultBotIPRanges(),
		BotUserAgents:       getDefaultBotUserAgents(),
		AllowedReferrers:    getDefaultAllowedReferrers(),
		EnableReverseDNS:    false,
		EnableReferrerCheck: true,
		EnableMetrics:       true,
		EnableRateLimit:     true,
		EnableDebug:         false,
		EmptyPageTemplate:   "",
		CacheTTL:            1 * time.Hour,
		DNSTimeout:          5 * time.Second,
		MaxDNSPerSecond:     10,
		MaxRequestsPerIP:    100,
		RateLimitWindow:     1 * time.Minute,
		MaxCacheSize:        10000,
		CleanupInterval:     10 * time.Minute,
		DNSWorkerPoolSize:   5,
		DNSQueueSize:        1000,
		LogLevel:            "info",
		LogAllRequests:      false,
		LogDNSQueries:       false,
		LogCacheOps:         false,
		VerboseMetrics:      false,
		MetricsPath:         "/metrics",
		EnablePrometheus:    false,
	}
}

