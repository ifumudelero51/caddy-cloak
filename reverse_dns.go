package botredirect

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// ReverseDNSChecker отвечает за асинхронную проверку обратного DNS
type ReverseDNSChecker struct {
	// Конфигурация
	enabled    bool
	timeout    time.Duration
	maxWorkers int
	queueSize  int

	// DNS resolver
	resolver *net.Resolver

	// Worker pool для асинхронных запросов
	jobQueue    chan *DNSJob
	resultQueue chan *DNSResult
	workers     []*DNSWorker

	// Кеш результатов
	cache    map[string]*DNSCheckResult
	cacheTTL time.Duration
	maxCache int

	// Паттерны для проверки доменов ботов
	botDomainPatterns map[BotType][]*regexp.Regexp

	// Синхронизация
	mutex  sync.RWMutex
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc

	// Компоненты
	metrics *Metrics
	debug   *DebugConfig
	logger  *zap.Logger

	// Статистика (используем atomic для thread-safety)
	totalRequests     int64
	successfulLookups int64
	failedLookups     int64
	timeouts          int64
	cacheHits         int64
	validBots         int64
	invalidBots       int64
}

// DNSJob представляет задачу для DNS worker'а
type DNSJob struct {
	ID         string
	IP         string
	RequestID  string
	StartTime  time.Time
	ResultChan chan *DNSResult
}

// DNSResult содержит результат DNS запроса
type DNSResult struct {
	Job        *DNSJob
	Hostname   string
	VerifiedIP string
	IsValid    bool
	BotType    BotType
	Error      error
	Duration   time.Duration
	Timestamp  time.Time
}

// DNSCheckResult содержит финальный результат проверки DNS
type DNSCheckResult struct {
	IsBot        bool
	Hostname     string
	VerifiedIP   string
	BotType      BotType
	Organization string
	Confidence   float64
	Error        string
	Duration     time.Duration
	Timestamp    time.Time
}

// DNSWorker выполняет DNS запросы в отдельной горутине
type DNSWorker struct {
	id      int
	checker *ReverseDNSChecker
	jobChan <-chan *DNSJob
	quit    chan bool
	logger  *zap.Logger
}

// NewReverseDNSChecker создает новый экземпляр ReverseDNSChecker
func NewReverseDNSChecker(config *Config, metrics *Metrics, debug *DebugConfig, logger *zap.Logger) *ReverseDNSChecker {
	if !config.EnableReverseDNS {
		return &ReverseDNSChecker{enabled: false}
	}

	ctx, cancel := context.WithCancel(context.Background())

	rdns := &ReverseDNSChecker{
		enabled:     true,
		timeout:     config.DNSTimeout,
		maxWorkers:  config.DNSWorkerPoolSize,
		queueSize:   config.DNSQueueSize,
		resolver:    &net.Resolver{},
		jobQueue:    make(chan *DNSJob, config.DNSQueueSize),
		resultQueue: make(chan *DNSResult, config.DNSQueueSize),
		cache:       make(map[string]*DNSCheckResult),
		cacheTTL:    config.CacheTTL,
		maxCache:    2000, // Кеш для 2000 DNS результатов
		ctx:         ctx,
		cancel:      cancel,
		metrics:     metrics,
		debug:       debug,
		logger:      logger,
	}

	// Инициализация паттернов доменов ботов
	rdns.initializeBotDomainPatterns()

	// Запуск worker pool
	rdns.startWorkerPool()

	// Запуск обработчика результатов
	go rdns.processResults()

	// Запуск периодической очистки кеша
	go rdns.startCacheCleanup()

	logger.Info("reverse DNS checker initialized",
		zap.Bool("enabled", true),
		zap.Duration("timeout", rdns.timeout),
		zap.Int("max_workers", rdns.maxWorkers),
		zap.Int("queue_size", rdns.queueSize),
		zap.Int("bot_domain_patterns", len(rdns.botDomainPatterns)),
	)

	return rdns
}

// initializeBotDomainPatterns инициализирует паттерны доменов для различных ботов
func (rdns *ReverseDNSChecker) initializeBotDomainPatterns() {
	rdns.botDomainPatterns = make(map[BotType][]*regexp.Regexp)

	// Паттерны для поисковых ботов
	searchPatterns := []string{
		`.*\.googlebot\.com$`,
		`.*\.google\.com$`,
		`.*\.search\.msn\.com$`,
		`.*\.crawl\.yahoo\.net$`,
		`.*\.yandex\.ru$`,
		`.*\.yandex\.net$`,
		`.*\.yandex\.com$`,
		`.*\.crawl\.baidu\.com$`,
		`.*\.crawl\.baidu\.jp$`,
		`.*\.spider\.sogou\.com$`,
		`.*\.crawl\.duckduckgo\.com$`,
	}

	// Паттерны для социальных сетей
	socialPatterns := []string{
		`.*\.facebook\.com$`,
		`.*\.fbsb\.com$`,
		`.*\.tfbnw\.net$`,
		`.*\.twitter\.com$`,
		`.*\.x\.com$`,
		`.*\.linkedin\.com$`,
		`.*\.whatsapp\.net$`,
		`.*\.telegram\.org$`,
		`.*\.apple\.com$`,
	}

	// Паттерны для SEO инструментов
	seoPatterns := []string{
		`.*\.ahrefs\.com$`,
		`.*\.semrush\.com$`,
		`.*\.moz\.com$`,
		`.*\.majestic12\.co\.uk$`,
		`.*\.screamingfrog\.co\.uk$`,
		`.*\.sistrix\.com$`,
		`.*\.serpstat\.com$`,
	}

	// Паттерны для мониторинга
	monitoringPatterns := []string{
		`.*\.pingdom\.com$`,
		`.*\.uptimerobot\.com$`,
		`.*\.statuscake\.com$`,
		`.*\.site24x7\.com$`,
		`.*\.monitor\.com$`,
		`.*\.alertsite\.com$`,
	}

	// Компилируем все паттерны
	rdns.botDomainPatterns[BotTypeSearch] = rdns.compilePatterns(searchPatterns)
	rdns.botDomainPatterns[BotTypeSocial] = rdns.compilePatterns(socialPatterns)
	rdns.botDomainPatterns[BotTypeSEO] = rdns.compilePatterns(seoPatterns)
	rdns.botDomainPatterns[BotTypeMonitoring] = rdns.compilePatterns(monitoringPatterns)
}

// compilePatterns компилирует список regex паттернов
func (rdns *ReverseDNSChecker) compilePatterns(patterns []string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		if regex, err := regexp.Compile(pattern); err == nil {
			compiled = append(compiled, regex)
		} else {
			rdns.logger.Warn("failed to compile DNS pattern",
				zap.String("pattern", pattern),
				zap.Error(err),
			)
		}
	}
	return compiled
}

// startWorkerPool запускает пул worker'ов для обработки DNS запросов
func (rdns *ReverseDNSChecker) startWorkerPool() {
	rdns.workers = make([]*DNSWorker, rdns.maxWorkers)

	for i := 0; i < rdns.maxWorkers; i++ {
		worker := &DNSWorker{
			id:      i,
			checker: rdns,
			jobChan: rdns.jobQueue,
			quit:    make(chan bool, 1), // ИСПРАВЛЕНИЕ: буферизованный канал
			logger:  rdns.logger.With(zap.Int("worker_id", i)),
		}

		rdns.workers[i] = worker
		rdns.wg.Add(1)
		go worker.start()
	}

	rdns.logger.Info("DNS worker pool started",
		zap.Int("workers", rdns.maxWorkers),
	)
}

// CheckDNS выполняет асинхронную проверку обратного DNS
func (rdns *ReverseDNSChecker) CheckDNS(ip string) (*DNSCheckResult, error) {
	if !rdns.enabled {
		return &DNSCheckResult{IsBot: false}, nil
	}

	if ip == "" {
		return &DNSCheckResult{IsBot: false}, nil
	}

	// Извлекаем чистый IP
	cleanIP := rdns.extractIP(ip)

	// Инкремент статистики
	atomic.AddInt64(&rdns.totalRequests, 1)

	// Проверка rate limiting для DNS запросов
	if rdns.metrics != nil {
		rdns.metrics.IncrementDNSRequests()
	}

	// Проверка кеша
	if result := rdns.getCachedResult(cleanIP); result != nil {
		atomic.AddInt64(&rdns.cacheHits, 1)
		if rdns.metrics != nil {
			rdns.metrics.IncrementCacheHits()
		}

		if rdns.debug != nil {
			rdns.debug.LogReverseDNSCheck(cleanIP, result.Hostname, result.IsBot, result.VerifiedIP)
		}

		return result, nil
	}

	if rdns.metrics != nil {
		rdns.metrics.IncrementCacheMisses()
	}

	// Создаем асинхронную задачу
	job := &DNSJob{
		ID:         fmt.Sprintf("dns_%d_%s", time.Now().UnixNano(), cleanIP),
		IP:         cleanIP,
		RequestID:  fmt.Sprintf("req_%d", time.Now().UnixNano()),
		StartTime:  time.Now(),
		ResultChan: make(chan *DNSResult, 1),
	}

	// Отправляем задачу в очередь (неблокирующая отправка)
	select {
	case rdns.jobQueue <- job:
		// Задача отправлена успешно
	case <-time.After(100 * time.Millisecond):
		// Очередь переполнена - возвращаем кешированный отрицательный результат
		result := &DNSCheckResult{
			IsBot:     false,
			Error:     "DNS queue full",
			Timestamp: time.Now(),
		}
		rdns.setCachedResult(cleanIP, result)
		return result, nil
	}

	// Ожидаем результат с таймаутом
	select {
	case dnsResult := <-job.ResultChan:
		result := rdns.processDNSResult(dnsResult)
		rdns.setCachedResult(cleanIP, result)

		if rdns.debug != nil {
			rdns.debug.LogReverseDNSCheck(cleanIP, result.Hostname, result.IsBot, result.VerifiedIP)
		}

		return result, nil

	case <-time.After(rdns.timeout):
		atomic.AddInt64(&rdns.timeouts, 1)
		if rdns.metrics != nil {
			rdns.metrics.IncrementDNSTimeouts()
		}

		result := &DNSCheckResult{
			IsBot:     false,
			Error:     "DNS timeout",
			Duration:  rdns.timeout,
			Timestamp: time.Now(),
		}
		rdns.setCachedResult(cleanIP, result)
		return result, nil
	}
}

// processDNSResult обрабатывает результат DNS запроса
func (rdns *ReverseDNSChecker) processDNSResult(dnsResult *DNSResult) *DNSCheckResult {
	if dnsResult.Error != nil {
		atomic.AddInt64(&rdns.failedLookups, 1)
		if rdns.metrics != nil {
			rdns.metrics.IncrementDNSErrors()
		}

		return &DNSCheckResult{
			IsBot:     false,
			Error:     dnsResult.Error.Error(),
			Duration:  dnsResult.Duration,
			Timestamp: dnsResult.Timestamp,
		}
	}

	atomic.AddInt64(&rdns.successfulLookups, 1)
	if rdns.metrics != nil {
		rdns.metrics.IncrementDNSSuccesses()
	}

	// Проверяем валидность результата
	if dnsResult.IsValid {
		atomic.AddInt64(&rdns.validBots, 1)
		return &DNSCheckResult{
			IsBot:        true,
			Hostname:     dnsResult.Hostname,
			VerifiedIP:   dnsResult.VerifiedIP,
			BotType:      dnsResult.BotType,
			Organization: rdns.getOrganizationByBotType(dnsResult.BotType),
			Confidence:   0.95, // Высокая уверенность для проверенного DNS
			Duration:     dnsResult.Duration,
			Timestamp:    dnsResult.Timestamp,
		}
	} else {
		atomic.AddInt64(&rdns.invalidBots, 1)
		return &DNSCheckResult{
			IsBot:     false,
			Hostname:  dnsResult.Hostname,
			Duration:  dnsResult.Duration,
			Timestamp: dnsResult.Timestamp,
		}
	}
}

// processResults обрабатывает результаты DNS запросов
func (rdns *ReverseDNSChecker) processResults() {
	for {
		select {
		case <-rdns.ctx.Done():
			return
		case result, ok := <-rdns.resultQueue:
			if !ok {
				return // канал закрыт
			}
			// Отправляем результат обратно в job
			if result.Job.ResultChan != nil {
				select {
				case result.Job.ResultChan <- result:
				case <-time.After(1 * time.Second):
					// Канал заблокирован - логируем и пропускаем
					rdns.logger.Warn("failed to send DNS result - channel blocked",
						zap.String("job_id", result.Job.ID),
					)
				}
			}
		}
	}
}

// start запускает DNS worker
func (worker *DNSWorker) start() {
	defer worker.checker.wg.Done()

	for {
		select {
		case <-worker.checker.ctx.Done():
			return
		case <-worker.quit:
			return
		case job, ok := <-worker.jobChan:
			if !ok {
				return // канал закрыт
			}
			worker.processJob(job)
		}
	}
}

// processJob обрабатывает DNS задачу
func (worker *DNSWorker) processJob(job *DNSJob) {
	startTime := time.Now()

	// Выполняем обратный DNS lookup
	hostname, err := worker.checker.lookupHostname(job.IP)
	if err != nil {
		result := &DNSResult{
			Job:       job,
			Error:     err,
			Duration:  time.Since(startTime),
			Timestamp: time.Now(),
		}
		worker.sendResult(result)
		return
	}

	// Проверяем прямой DNS lookup для верификации
	verifiedIP, err := worker.checker.lookupIP(hostname)
	if err != nil {
		result := &DNSResult{
			Job:       job,
			Hostname:  hostname,
			IsValid:   false,
			Error:     err,
			Duration:  time.Since(startTime),
			Timestamp: time.Now(),
		}
		worker.sendResult(result)
		return
	}

	// Проверяем соответствие IP адресов
	isValid := worker.checker.verifyIPMatch(job.IP, verifiedIP)
	botType := worker.checker.determineBotTypeByHostname(hostname)

	result := &DNSResult{
		Job:        job,
		Hostname:   hostname,
		VerifiedIP: verifiedIP,
		IsValid:    isValid && botType != BotTypeUnknown,
		BotType:    botType,
		Duration:   time.Since(startTime),
		Timestamp:  time.Now(),
	}

	worker.sendResult(result)

	// Логирование для дебага
	if worker.checker.debug != nil {
		worker.checker.debug.LogDNSQuery(&DNSDebugInfo{
			IP:        job.IP,
			Hostname:  hostname,
			QueryType: "PTR+A",
			Result:    verifiedIP,
			Duration:  result.Duration,
			Timestamp: result.Timestamp,
		})
	}
}

// sendResult отправляет результат в очередь
func (worker *DNSWorker) sendResult(result *DNSResult) {
	select {
	case worker.checker.resultQueue <- result:
	case <-time.After(1 * time.Second):
		worker.logger.Warn("failed to send result to queue - queue full",
			zap.String("job_id", result.Job.ID),
		)
	}
}

// lookupHostname выполняет обратный DNS lookup (PTR запрос)
func (rdns *ReverseDNSChecker) lookupHostname(ip string) (string, error) {
	ctx, cancel := context.WithTimeout(rdns.ctx, rdns.timeout)
	defer cancel()

	hostnames, err := rdns.resolver.LookupAddr(ctx, ip)
	if err != nil {
		return "", fmt.Errorf("PTR lookup failed: %w", err)
	}

	if len(hostnames) == 0 {
		return "", fmt.Errorf("no PTR records found")
	}

	// Возвращаем первый hostname (обычно самый релевантный)
	hostname := strings.TrimRight(hostnames[0], ".")
	return hostname, nil
}

// lookupIP выполняет прямой DNS lookup (A/AAAA запрос)
func (rdns *ReverseDNSChecker) lookupIP(hostname string) (string, error) {
	ctx, cancel := context.WithTimeout(rdns.ctx, rdns.timeout)
	defer cancel()

	ips, err := rdns.resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return "", fmt.Errorf("A/AAAA lookup failed: %w", err)
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no A/AAAA records found")
	}

	// Возвращаем первый IP
	return ips[0].IP.String(), nil
}

// verifyIPMatch проверяет соответствие исходного и проверенного IP
func (rdns *ReverseDNSChecker) verifyIPMatch(originalIP, verifiedIP string) bool {
	// Простое сравнение строк для точного совпадения
	if originalIP == verifiedIP {
		return true
	}

	// Парсим IP адреса для более точного сравнения
	origIP := net.ParseIP(originalIP)
	verIP := net.ParseIP(verifiedIP)

	if origIP == nil || verIP == nil {
		return false
	}

	// Проверяем равенство
	return origIP.Equal(verIP)
}

// determineBotTypeByHostname определяет тип бота по hostname
func (rdns *ReverseDNSChecker) determineBotTypeByHostname(hostname string) BotType {
	hostname = strings.ToLower(hostname)

	for botType, patterns := range rdns.botDomainPatterns {
		for _, pattern := range patterns {
			if pattern.MatchString(hostname) {
				return botType
			}
		}
	}

	return BotTypeUnknown
}

// getOrganizationByBotType возвращает организацию по типу бота
func (rdns *ReverseDNSChecker) getOrganizationByBotType(botType BotType) string {
	orgMap := map[BotType]string{
		BotTypeSearch:     "Search Engine",
		BotTypeSocial:     "Social Media",
		BotTypeSEO:        "SEO Tool",
		BotTypeMonitoring: "Monitoring Service",
		BotTypeCrawler:    "Web Crawler",
	}

	if org, exists := orgMap[botType]; exists {
		return org
	}
	return "Unknown"
}

// extractIP извлекает IP адрес из строки (убирает порт)
func (rdns *ReverseDNSChecker) extractIP(address string) string {
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

// Методы для работы с кешем
func (rdns *ReverseDNSChecker) getCachedResult(ip string) *DNSCheckResult {
	rdns.mutex.RLock()
	defer rdns.mutex.RUnlock()

	if result, exists := rdns.cache[ip]; exists {
		// Проверка TTL
		if time.Since(result.Timestamp) < rdns.cacheTTL {
			return result
		}
		// Удаление устаревшей записи
		delete(rdns.cache, ip)
	}

	return nil
}

func (rdns *ReverseDNSChecker) setCachedResult(ip string, result *DNSCheckResult) {
	rdns.mutex.Lock()
	defer rdns.mutex.Unlock()

	// Проверка размера кеша
	if len(rdns.cache) >= rdns.maxCache {
		rdns.cleanupCacheUnsafe()
	}

	rdns.cache[ip] = result
}

func (rdns *ReverseDNSChecker) cleanupCacheUnsafe() {
	now := time.Now()

	for key, result := range rdns.cache {
		if now.Sub(result.Timestamp) > rdns.cacheTTL {
			delete(rdns.cache, key)
		}
	}

	// Если кеш все еще переполнен, удаляем самые старые записи
	if len(rdns.cache) >= rdns.maxCache {
		count := 0
		target := len(rdns.cache) / 2

		for key := range rdns.cache {
			if count >= target {
				break
			}
			delete(rdns.cache, key)
			count++
		}
	}
}

// startCacheCleanup запускает периодическую очистку кеша
func (rdns *ReverseDNSChecker) startCacheCleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rdns.ctx.Done():
			return
		case <-ticker.C:
			rdns.mutex.Lock()
			rdns.cleanupCacheUnsafe()
			rdns.mutex.Unlock()
		}
	}
}

// GetStats возвращает статистику
func (rdns *ReverseDNSChecker) GetStats() map[string]interface{} {
	if !rdns.enabled {
		return map[string]interface{}{"enabled": false}
	}

	rdns.mutex.RLock()
	cacheSize := len(rdns.cache)
	queueSize := len(rdns.jobQueue)
	rdns.mutex.RUnlock()

	totalRequests := atomic.LoadInt64(&rdns.totalRequests)
	successfulLookups := atomic.LoadInt64(&rdns.successfulLookups)
	cacheHits := atomic.LoadInt64(&rdns.cacheHits)
	validBots := atomic.LoadInt64(&rdns.validBots)

	successRate := 0.0
	if totalRequests > 0 {
		successRate = float64(successfulLookups) / float64(totalRequests)
	}

	cacheHitRate := 0.0
	if totalRequests > 0 {
		cacheHitRate = float64(cacheHits) / float64(totalRequests)
	}

	validBotRate := 0.0
	if successfulLookups > 0 {
		validBotRate = float64(validBots) / float64(successfulLookups)
	}

	return map[string]interface{}{
		"enabled":            true,
		"total_requests":     totalRequests,
		"successful_lookups": successfulLookups,
		"failed_lookups":     atomic.LoadInt64(&rdns.failedLookups),
		"timeouts":           atomic.LoadInt64(&rdns.timeouts),
		"cache_hits":         cacheHits,
		"valid_bots":         validBots,
		"invalid_bots":       atomic.LoadInt64(&rdns.invalidBots),
		"success_rate":       successRate,
		"cache_hit_rate":     cacheHitRate,
		"valid_bot_rate":     validBotRate,
		"cache_size":         cacheSize,
		"cache_max_size":     rdns.maxCache,
		"worker_count":       len(rdns.workers),
		"queue_size":         queueSize,
		"bot_patterns":       len(rdns.botDomainPatterns),
	}
}

// Shutdown gracefully останавливает сервис
func (rdns *ReverseDNSChecker) Shutdown() {
	if !rdns.enabled {
		return
	}

	rdns.logger.Info("shutting down reverse DNS checker")

	// Останавливаем контекст
	rdns.cancel()

	// ИСПРАВЛЕНИЕ: Безопасная остановка worker'ов
	for _, worker := range rdns.workers {
		select {
		case worker.quit <- true:
		default:
		}
	}

	// Ждем завершения всех worker'ов
	rdns.wg.Wait()

	// ИСПРАВЛЕНИЕ: Безопасно закрываем каналы
	select {
	case <-rdns.jobQueue:
	default:
		close(rdns.jobQueue)
	}

	select {
	case <-rdns.resultQueue:
	default:
		close(rdns.resultQueue)
	}

	rdns.logger.Info("reverse DNS checker shutdown completed")
}

// ClearCache очищает кеш DNS результатов
func (rdns *ReverseDNSChecker) ClearCache() {
	if !rdns.enabled {
		return
	}

	rdns.mutex.Lock()
	rdns.cache = make(map[string]*DNSCheckResult)
	rdns.mutex.Unlock()

	rdns.logger.Info("reverse DNS checker cache cleared")
}

// AddBotDomainPattern добавляет новый паттерн домена бота
func (rdns *ReverseDNSChecker) AddBotDomainPattern(botType BotType, pattern string) error {
	if !rdns.enabled {
		return nil
	}

	if pattern == "" {
		return fmt.Errorf("empty pattern")
	}

	regex, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern %s: %w", pattern, err)
	}

	rdns.mutex.Lock()
	if rdns.botDomainPatterns[botType] == nil {
		rdns.botDomainPatterns[botType] = make([]*regexp.Regexp, 0)
	}
	rdns.botDomainPatterns[botType] = append(rdns.botDomainPatterns[botType], regex)
	rdns.mutex.Unlock()

	rdns.logger.Info("added new bot domain pattern",
		zap.String("bot_type", string(botType)),
		zap.String("pattern", pattern),
	)

	return nil
}

// IsEnabled возвращает статус включенности reverse DNS checker
func (rdns *ReverseDNSChecker) IsEnabled() bool {
	return rdns.enabled
}

// UpdateTimeout обновляет таймаут DNS запросов
func (rdns *ReverseDNSChecker) UpdateTimeout(timeout time.Duration) {
	if !rdns.enabled {
		return
	}

	rdns.mutex.Lock()
	rdns.timeout = timeout
	rdns.mutex.Unlock()

	rdns.logger.Info("DNS timeout updated",
		zap.Duration("new_timeout", timeout),
	)
}
