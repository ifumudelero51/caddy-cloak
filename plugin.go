package botredirect

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

func init() {
	caddy.RegisterModule(BotRedirect{})
	httpcaddyfile.RegisterHandlerDirective("bot_redirect", parseCaddyfile)
}

// BotRedirect реализует HTTP middleware для разделения ботов и пользователей
type BotRedirect struct {
	// Конфигурационные поля
	RedirectURL         string         `json:"redirect_url,omitempty"`
	BotIPRanges         []string       `json:"bot_ip_ranges,omitempty"`
	BotUserAgents       []string       `json:"bot_user_agents,omitempty"`
	AllowedReferrers    []string       `json:"allowed_referrers,omitempty"`
	EnableReverseDNS    bool           `json:"enable_reverse_dns,omitempty"`
	EnableReferrerCheck bool           `json:"enable_referrer_check,omitempty"`
	EnableMetrics       bool           `json:"enable_metrics,omitempty"`
	EnableRateLimit     bool           `json:"enable_rate_limit,omitempty"`
	EnableDebug         bool           `json:"enable_debug,omitempty"`
	EmptyPageTemplate   string         `json:"empty_page_template,omitempty"`
	CacheTTL            caddy.Duration `json:"cache_ttl,omitempty"`
	DNSTimeout          caddy.Duration `json:"dns_timeout,omitempty"`
	MaxDNSPerSecond     int            `json:"max_dns_per_second,omitempty"`
	MaxRequestsPerIP    int            `json:"max_requests_per_ip,omitempty"`
	RateLimitWindow     caddy.Duration `json:"rate_limit_window,omitempty"`
	MaxCacheSize        int            `json:"max_cache_size,omitempty"`
	CleanupInterval     caddy.Duration `json:"cleanup_interval,omitempty"`
	DNSWorkerPoolSize   int            `json:"dns_worker_pool_size,omitempty"`
	DNSQueueSize        int            `json:"dns_queue_size,omitempty"`
	LogLevel            string         `json:"log_level,omitempty"`
	LogAllRequests      bool           `json:"log_all_requests,omitempty"`
	LogDNSQueries       bool           `json:"log_dns_queries,omitempty"`
	LogCacheOps         bool           `json:"log_cache_ops,omitempty"`
	VerboseMetrics      bool           `json:"verbose_metrics,omitempty"`
	MetricsPath         string         `json:"metrics_path,omitempty"`
	EnablePrometheus    bool           `json:"enable_prometheus,omitempty"`

	// Главный компонент
	botDetector *BotDetector `json:"-"`

	// Логгер
	logger *zap.Logger `json:"-"`
}

// CaddyModule возвращает информацию о модуле Caddy
func (BotRedirect) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.bot_redirect",
		New: func() caddy.Module { return new(BotRedirect) },
	}
}

// Provision настраивает модуль во время инициализации
func (br *BotRedirect) Provision(ctx caddy.Context) error {
	br.logger = ctx.Logger()

	// ИСПРАВЛЕНИЕ: Валидация перед инициализацией
	if br.RedirectURL == "" {
		return fmt.Errorf("bot_redirect: redirect_url is required")
	}

	// Установка значений по умолчанию
	if br.CacheTTL == 0 {
		br.CacheTTL = caddy.Duration(1 * time.Hour)
	}

	if br.DNSTimeout == 0 {
		br.DNSTimeout = caddy.Duration(5 * time.Second)
	}

	if br.LogLevel == "" {
		br.LogLevel = "info"
	}

	if br.MaxDNSPerSecond == 0 {
		br.MaxDNSPerSecond = 10
	}

	if br.MaxRequestsPerIP == 0 {
		br.MaxRequestsPerIP = 100
	}

	if br.RateLimitWindow == 0 {
		br.RateLimitWindow = caddy.Duration(1 * time.Minute)
	}

	if br.DNSWorkerPoolSize == 0 {
		br.DNSWorkerPoolSize = 5
	}

	if br.DNSQueueSize == 0 {
		br.DNSQueueSize = 1000
	}

	if br.MaxCacheSize == 0 {
		br.MaxCacheSize = 10000
	}

	if br.CleanupInterval == 0 {
		br.CleanupInterval = caddy.Duration(10 * time.Minute)
	}

	if br.MetricsPath == "" {
		br.MetricsPath = "/metrics"
	}

	// Создание конфигурации
	config := &Config{
		RedirectURL:         br.RedirectURL,
		BotIPRanges:         br.BotIPRanges,
		BotUserAgents:       br.BotUserAgents,
		AllowedReferrers:    br.AllowedReferrers,
		EnableReverseDNS:    br.EnableReverseDNS,
		EnableReferrerCheck: br.EnableReferrerCheck,
		EnableMetrics:       br.EnableMetrics,
		EnableRateLimit:     br.EnableRateLimit,
		EnableDebug:         br.EnableDebug,
		EmptyPageTemplate:   br.EmptyPageTemplate,
		CacheTTL:            time.Duration(br.CacheTTL),
		DNSTimeout:          time.Duration(br.DNSTimeout),
		MaxDNSPerSecond:     br.MaxDNSPerSecond,
		MaxRequestsPerIP:    br.MaxRequestsPerIP,
		RateLimitWindow:     time.Duration(br.RateLimitWindow),
		MaxCacheSize:        br.MaxCacheSize,
		CleanupInterval:     time.Duration(br.CleanupInterval),
		DNSWorkerPoolSize:   br.DNSWorkerPoolSize,
		DNSQueueSize:        br.DNSQueueSize,
		LogLevel:            br.LogLevel,
		LogAllRequests:      br.LogAllRequests,
		LogDNSQueries:       br.LogDNSQueries,
		LogCacheOps:         br.LogCacheOps,
		VerboseMetrics:      br.VerboseMetrics,
		MetricsPath:         br.MetricsPath,
		EnablePrometheus:    br.EnablePrometheus,
	}

	// Дополнительная валидация конфигурации
	if err := br.validateConfig(config); err != nil {
		return fmt.Errorf("bot_redirect: invalid configuration: %w", err)
	}

	// Инициализация главного компонента
	br.botDetector = NewBotDetector(config, br.logger)

	br.logger.Info("bot_redirect plugin provisioned",
		zap.String("redirect_url", br.RedirectURL),
		zap.Bool("enable_reverse_dns", br.EnableReverseDNS),
		zap.Bool("enable_referrer_check", br.EnableReferrerCheck),
		zap.Bool("enable_metrics", br.EnableMetrics),
		zap.Bool("enable_rate_limit", br.EnableRateLimit),
		zap.Bool("enable_debug", br.EnableDebug),
		zap.Duration("cache_ttl", time.Duration(br.CacheTTL)),
		zap.Int("max_dns_per_second", br.MaxDNSPerSecond),
		zap.Int("max_requests_per_ip", br.MaxRequestsPerIP),
	)

	return nil
}

// Validate проверяет конфигурацию модуля (теперь используется только для финальной проверки)
func (br *BotRedirect) Validate() error {
	return nil // Основная валидация перенесена в Provision
}

// ServeHTTP обрабатывает HTTP запросы
func (br *BotRedirect) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	startTime := time.Now()

	// Проверка rate limiting для общих запросов
	rateLimiter := br.botDetector.GetRateLimiter()
	if rateLimiter != nil && !rateLimiter.CheckRequest(r.RemoteAddr) {
		metrics := br.botDetector.GetMetrics()
		if metrics != nil {
			metrics.IncrementRateLimitBlocked()
		}
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return nil
	}

	// Определение типа пользователя через BotDetector
	detectionResult := br.botDetector.DetectBot(r)

	br.logger.Debug("request processed",
		zap.String("ip", r.RemoteAddr),
		zap.String("user_agent", r.UserAgent()),
		zap.String("referer", r.Referer()),
		zap.String("user_type", detectionResult.UserType.String()),
		zap.String("detection_method", detectionResult.DetectionMethod),
		zap.Float64("confidence", detectionResult.Confidence),
		zap.Duration("processing_time", detectionResult.ProcessingTime),
	)

	var err error

	switch detectionResult.UserType {
	case UserTypeBot:
		// Боты - показываем оригинальный контент
		err = next.ServeHTTP(w, r)

	case UserTypeFromSearch:
		// Пользователи с поисковиков - редирект
		http.Redirect(w, r, br.RedirectURL, http.StatusFound)

	case UserTypeDirect:
		// Прямые заходы - пустая страница
		templates := br.botDetector.GetTemplates()
		if templates != nil {
			err = templates.ServeEmptyPage(w, r)
		} else {
			err = br.serveDefaultEmptyPage(w, r)
		}

	default:
		err = next.ServeHTTP(w, r)
	}

	// Запись времени обработки в метрики
	processingTime := time.Since(startTime)
	metrics := br.botDetector.GetMetrics()
	if metrics != nil {
		metrics.RecordProcessingTime(processingTime)
	}

	return err
}

// serveDefaultEmptyPage отдает базовую пустую HTML страницу
func (br *BotRedirect) serveDefaultEmptyPage(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(http.StatusOK)

	template := br.EmptyPageTemplate
	if template == "" {
		template = getDefaultEmptyPageTemplate()
	}

	_, err := w.Write([]byte(template))
	return err
}

// validateConfig проверяет корректность конфигурации
func (br *BotRedirect) validateConfig(config *Config) error {
	if config.CacheTTL < 0 {
		return fmt.Errorf("cache_ttl must be positive")
	}

	if config.DNSTimeout < 0 {
		return fmt.Errorf("dns_timeout must be positive")
	}

	if config.MaxDNSPerSecond < 0 {
		return fmt.Errorf("max_dns_per_second must be positive")
	}

	if config.MaxRequestsPerIP < 0 {
		return fmt.Errorf("max_requests_per_ip must be positive")
	}

	if config.RateLimitWindow < 0 {
		return fmt.Errorf("rate_limit_window must be positive")
	}

	if config.DNSWorkerPoolSize < 1 {
		return fmt.Errorf("dns_worker_pool_size must be at least 1")
	}

	if config.MaxCacheSize < 100 {
		return fmt.Errorf("max_cache_size must be at least 100")
	}

	return nil
}

// getDefaultEmptyPageTemplate возвращает базовый шаблон пустой страницы
func getDefaultEmptyPageTemplate() string {
	return `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Page Not Found</title>
    <meta name="robots" content="noindex, nofollow">
    <style>
        body { font-family: Arial, sans-serif; text-align: center; padding: 50px; }
        h1 { color: #666; }
    </style>
</head>
<body>
    <h1>404 - Page Not Found</h1>
    <p>The requested page could not be found.</p>
</body>
</html>`
}

// parseCaddyfile парсит конфигурацию из Caddyfile
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var br BotRedirect

	err := br.UnmarshalCaddyfile(h.Dispenser)
	if err != nil {
		return nil, err
	}

	return &br, nil
}

// UnmarshalCaddyfile реализует парсинг Caddyfile
func (br *BotRedirect) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "redirect_url":
				if !d.Args(&br.RedirectURL) {
					return d.ArgErr()
				}

			case "bot_ip_ranges":
				br.BotIPRanges = d.RemainingArgs()

			case "bot_user_agents":
				br.BotUserAgents = d.RemainingArgs()

			case "allowed_referrers":
				br.AllowedReferrers = d.RemainingArgs()

			case "enable_reverse_dns":
				if d.NextArg() {
					br.EnableReverseDNS = d.Val() == "true"
				} else {
					br.EnableReverseDNS = true
				}

			case "enable_referrer_check":
				if d.NextArg() {
					br.EnableReferrerCheck = d.Val() == "true"
				} else {
					br.EnableReferrerCheck = true
				}

			case "enable_metrics":
				if d.NextArg() {
					br.EnableMetrics = d.Val() == "true"
				} else {
					br.EnableMetrics = true
				}

			case "enable_rate_limit":
				if d.NextArg() {
					br.EnableRateLimit = d.Val() == "true"
				} else {
					br.EnableRateLimit = true
				}

			case "enable_debug":
				if d.NextArg() {
					br.EnableDebug = d.Val() == "true"
				} else {
					br.EnableDebug = true
				}

			case "empty_page_template":
				if !d.Args(&br.EmptyPageTemplate) {
					return d.ArgErr()
				}

			case "cache_ttl":
				var ttlStr string
				if !d.Args(&ttlStr) {
					return d.ArgErr()
				}

				ttl, err := time.ParseDuration(ttlStr)
				if err != nil {
					return d.Errf("invalid cache_ttl duration: %v", err)
				}
				br.CacheTTL = caddy.Duration(ttl)

			case "dns_timeout":
				var timeoutStr string
				if !d.Args(&timeoutStr) {
					return d.ArgErr()
				}

				timeout, err := time.ParseDuration(timeoutStr)
				if err != nil {
					return d.Errf("invalid dns_timeout duration: %v", err)
				}
				br.DNSTimeout = caddy.Duration(timeout)

			case "max_dns_per_second":
				var maxDNSStr string
				if !d.Args(&maxDNSStr) {
					return d.ArgErr()
				}

				maxDNS, err := strconv.Atoi(maxDNSStr)
				if err != nil {
					return d.Errf("invalid max_dns_per_second: %v", err)
				}
				br.MaxDNSPerSecond = maxDNS

			case "max_requests_per_ip":
				var maxReqStr string
				if !d.Args(&maxReqStr) {
					return d.ArgErr()
				}

				maxReq, err := strconv.Atoi(maxReqStr)
				if err != nil {
					return d.Errf("invalid max_requests_per_ip: %v", err)
				}
				br.MaxRequestsPerIP = maxReq

			case "rate_limit_window":
				var windowStr string
				if !d.Args(&windowStr) {
					return d.ArgErr()
				}

				window, err := time.ParseDuration(windowStr)
				if err != nil {
					return d.Errf("invalid rate_limit_window duration: %v", err)
				}
				br.RateLimitWindow = caddy.Duration(window)

			case "max_cache_size":
				var cacheStr string
				if !d.Args(&cacheStr) {
					return d.ArgErr()
				}

				cacheSize, err := strconv.Atoi(cacheStr)
				if err != nil {
					return d.Errf("invalid max_cache_size: %v", err)
				}
				br.MaxCacheSize = cacheSize

			case "cleanup_interval":
				var intervalStr string
				if !d.Args(&intervalStr) {
					return d.ArgErr()
				}

				interval, err := time.ParseDuration(intervalStr)
				if err != nil {
					return d.Errf("invalid cleanup_interval duration: %v", err)
				}
				br.CleanupInterval = caddy.Duration(interval)

			case "dns_worker_pool_size":
				var poolSizeStr string
				if !d.Args(&poolSizeStr) {
					return d.ArgErr()
				}

				poolSize, err := strconv.Atoi(poolSizeStr)
				if err != nil {
					return d.Errf("invalid dns_worker_pool_size: %v", err)
				}
				br.DNSWorkerPoolSize = poolSize

			case "dns_queue_size":
				var queueSizeStr string
				if !d.Args(&queueSizeStr) {
					return d.ArgErr()
				}

				queueSize, err := strconv.Atoi(queueSizeStr)
				if err != nil {
					return d.Errf("invalid dns_queue_size: %v", err)
				}
				br.DNSQueueSize = queueSize

			case "log_level":
				if !d.Args(&br.LogLevel) {
					return d.ArgErr()
				}

			case "log_all_requests":
				if d.NextArg() {
					br.LogAllRequests = d.Val() == "true"
				} else {
					br.LogAllRequests = true
				}

			case "log_dns_queries":
				if d.NextArg() {
					br.LogDNSQueries = d.Val() == "true"
				} else {
					br.LogDNSQueries = true
				}

			case "log_cache_ops":
				if d.NextArg() {
					br.LogCacheOps = d.Val() == "true"
				} else {
					br.LogCacheOps = true
				}

			case "verbose_metrics":
				if d.NextArg() {
					br.VerboseMetrics = d.Val() == "true"
				} else {
					br.VerboseMetrics = true
				}

			case "metrics_path":
				if !d.Args(&br.MetricsPath) {
					return d.ArgErr()
				}

			case "enable_prometheus":
				if d.NextArg() {
					br.EnablePrometheus = d.Val() == "true"
				} else {
					br.EnablePrometheus = true
				}

			default:
				return d.Errf("unknown directive: %s", d.Val())
			}
		}
	}

	return nil
}

// GetBotDetector возвращает экземпляр BotDetector для доступа к статистике
func (br *BotRedirect) GetBotDetector() *BotDetector {
	return br.botDetector
}

// Cleanup очистка ресурсов при завершении работы
func (br *BotRedirect) Cleanup() error {
	if br.botDetector != nil {
		br.botDetector.Shutdown()
	}
	return nil
}

// Interface guards
var (
	_ caddy.Provisioner           = (*BotRedirect)(nil)
	_ caddy.Validator             = (*BotRedirect)(nil)
	_ caddyhttp.MiddlewareHandler = (*BotRedirect)(nil)
	_ caddyfile.Unmarshaler       = (*BotRedirect)(nil)
	_ caddy.CleanerUpper          = (*BotRedirect)(nil)
)