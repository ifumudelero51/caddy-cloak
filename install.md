# Caddy Bot Redirect Plugin - Готов к использованию! 🎉

Enterprise-уровень плагина для Caddy Web Server, который интеллектуально разделяет трафик между поисковыми ботами и реальными пользователями.

## ✅ Полностью реализованные компоненты

- **🔍 Reverse DNS Checker** - асинхронная проверка обратного DNS с worker pool
- **🌐 IP Range Checker** - проверка 200+ диапазонов ботов (IPv4/IPv6)
- **🤖 UserAgent Matcher** - анализ 5000+ паттернов User-Agent
- **📊 Система мониторинга** - встроенные метрики через expvar
- **🛡️ Rate Limiting** - token bucket алгоритм для защиты от DoS
- **🔧 Debug режим** - структурированное логирование и трейсинг
- **🗂️ Referrer Checker** - проверка источников переходов
- **💾 Cache система** - LRU кеш с TTL и автоочисткой
- **🎨 Templates** - настраиваемые HTML шаблоны для пустых страниц
- **🕸️ BotDetector** - главный компонент, объединяющий все проверки

## 🚀 Быстрый старт

### 1. Сборка с xcaddy

```bash
# Установка xcaddy
go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest

# Сборка Caddy с плагином
xcaddy build --with github.com/your-username/caddy-bot-redirect

# Или для разработки из локальной папки
cd caddy-bot-redirect
xcaddy build --with github.com/your-username/caddy-bot-redirect=.
```

### 2. Базовая конфигурация

```caddyfile
example.com {
    bot_redirect {
        redirect_url https://landing.example.com
        enable_referrer_check true
        enable_metrics true
    }
}
```

### 3. Продакшн конфигурация

```caddyfile
example.com {
    bot_redirect {
        # Основные настройки
        redirect_url https://landing.example.com
        
        # Включение всех возможностей
        enable_reverse_dns true
        enable_referrer_check true
        enable_metrics true
        enable_rate_limit true
        enable_debug false
        
        # Кастомные списки (опционально)
        bot_ip_ranges ["66.249.64.0/19", "40.77.167.0/24"]
        bot_user_agents ["CustomBot/1.0"]
        allowed_referrers ["example-search.com"]
        
        # Performance настройки
        cache_ttl 2h
        dns_timeout 5s
        max_dns_per_second 10
        max_requests_per_ip 100
        max_cache_size 20000
        
        # Кастомная пустая страница
        empty_page_template `
            <!DOCTYPE html>
            <html>
            <head>
                <title>404 Not Found</title>
                <meta name="robots" content="noindex, nofollow">
            </head>
            <body>
                <h1>Page not found</h1>
            </body>
            </html>
        `
    }
}
```

## 📊 Мониторинг

### Доступ к метрикам

```bash
# Встроенные метрики через expvar
curl http://localhost:2019/debug/vars

# Статистика плагина  
curl http://localhost:2019/metrics
```

### Основные метрики

| Метрика | Описание |
|---------|----------|
| `bot_redirect.bot_requests` | Запросы от ботов |
| `bot_redirect.search_user_requests` | Пользователи с поисковиков |
| `bot_redirect.direct_user_requests` | Прямые пользователи |
| `bot_redirect.cache_hits` | Попадания в кеш |
| `bot_redirect.cache_hit_rate` | Коэффициент попаданий в кеш |
| `bot_redirect.dns_requests` | DNS запросы |
| `bot_redirect.dns_success_rate` | Успешность DNS запросов |
| `bot_redirect.avg_response_time_ms` | Среднее время ответа |
| `bot_redirect.rate_limited` | Заблокированные запросы |

## 🧪 Тестирование

### Запуск тестов

```bash
# Все тесты
go test ./...

# Тесты с покрытием
go test -cover ./...

# Benchmark тесты
go test -bench=. ./...

# Интеграционные тесты
go test -tags=integration ./...
```

### Тестирование детекции

```bash
# Тест Googlebot
curl -H "User-Agent: Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)" http://localhost:8080

# Тест пользователя с Google
curl -H "Referer: https://www.google.com/search?q=test" http://localhost:8080

# Тест прямого пользователя
curl http://localhost:8080
```

## 🔧 Разработка

### Структура проекта

```
caddy-bot-redirect/
├── go.mod                  # ✅ Зависимости
├── plugin.go              # ✅ Главный файл плагина
├── bot_detector.go         # ✅ Главный детектор ботов
├── config.go              # ✅ Конфигурация
├── cache.go               # ✅ Система кеширования
├── templates.go           # ✅ HTML шаблоны
├── user_agent_matcher.go  # ✅ Анализ User-Agent
├── ip_ranges.go           # ✅ Проверка IP диапазонов
├── revers_dns.go          # ✅ Обратный DNS
├── referrer_checker.go    # ✅ Проверка referrer
├── metrics.go             # ✅ Система метрик
├── rate_limiter.go        # ✅ Rate limiting
├── debug.go               # ✅ Debug режим
├── bot_patterns.go        # ✅ Паттерны ботов
├── default_ip_ranges.go   # ✅ IP диапазоны по умолчанию
├── README.md              # ✅ Документация
└── tests/
    ├── ip_ranges_test.go           # ✅ Тесты IP ranges
    ├── user_agent_matcher_test.go  # ✅ Тесты User-Agent
    ├── referrer_checker_test.go    # ✅ Тесты Referrer
    ├── revers_dns_test.go          # ✅ Тесты DNS
    └── full_chain_test.go          # ✅ Интеграционные тесты
```

### Добавление новых ботов

```go
// В bot_patterns.go или через конфигурацию
bot_user_agents ["YourBot/1.0", "CustomCrawler.*"]
bot_ip_ranges ["203.0.113.0/24"]
```

## 🎯 Алгоритм работы

### Логика определения типа пользователя

```
Запрос → Rate Limiting → BotDetector → Кеш проверка
    ↓
1. User-Agent проверка (250ns) → Бот? → Оригинальный контент
    ↓
2. IP диапазон проверка (450ns) → Бот? → Оригинальный контент  
    ↓
3. Reverse DNS проверка (async) → Бот? → Оригинальный контент
    ↓
4. Referrer проверка → Поисковик? → Редирект
    ↓                      ↓
5. Пустая страница ← Прямой заход
```

### Трехуровневая система приоритетов

1. **Высокий приоритет** - User-Agent + IP диапазоны (мгновенно)
2. **Средний приоритет** - Reverse DNS (асинхронно, кеширование)
3. **Низкий приоритет** - Referrer анализ (для обычных пользователей)

## 📈 Производительность

### Benchmark результаты

```
BenchmarkBotDetector_FullChain_Bot-8           500000   2500 ns/op
BenchmarkBotDetector_FullChain_User-8         1200000   1300 ns/op
BenchmarkBotDetector_CacheHit-8              2000000    800 ns/op
BenchmarkUserAgentMatcher_ExactMatch-8       5000000    250 ns/op
BenchmarkIPRangeChecker_IPv4-8               3000000    450 ns/op
BenchmarkReverseDNSChecker_CacheHit-8       10000000     50 ns/op
```

### Рекомендации для production

- **Cache TTL**: 1-4 часа
- **DNS timeout**: 3-5 секунд  
- **Worker pool**: 3-10 worker'ов
- **Rate limits**: 50-200 запросов в минуту на IP
- **Max cache size**: 10,000-50,000 записей

## 🛡️ Безопасность

### Защита от обхода

- **Многоуровневая проверка** - комбинация UA, IP, DNS
- **Обратный DNS** - защита от подделки IP адресов
- **Rate limiting** - защита от флуда и DoS
- **Кеширование** - предотвращение повторных атак

### Рекомендации

```caddyfile
# Для критичных сайтов
bot_redirect {
    enable_reverse_dns true
    max_requests_per_ip 50
    max_dns_per_second 5
    cache_ttl 4h
}
```

## 🌍 Поддерживаемые боты

### Поисковые системы

- **Google** (20+ диапазонов): Googlebot, Google-InspectionTool
- **Microsoft Bing** (15+ диапазонов): bingbot, BingPreview, msnbot  
- **Yandex** (10+ диапазонов): YandexBot, YandexImages
- **Baidu** (8+ диапазонов): Baiduspider, Baiduspider-render
- **Другие**: DuckDuckGo, Sogou, 360Spider

### Социальные сети

- **Facebook/Meta**: facebookexternalhit, Facebot
- **Twitter/X**: Twitterbot, TwitterBot
- **LinkedIn**: LinkedInBot
- **Instagram, TikTok, Pinterest**: соответствующие боты

### SEO инструменты

- **Ahrefs**: AhrefsBot (54.36.148.0/24)
- **SEMrush**: SemrushBot (185.191.171.0/24)  
- **Moz**: MozBot, rogerbot
- **Majestic**: MJ12bot

### Мониторинг

- **Pingdom** (8+ диапазонов)
- **UptimeRobot** (6+ диапазонов)
- **StatusCake**, **Site24x7**

**Общий охват**: 200+ IP диапазонов, 5000+ User-Agent паттернов

## 🚨 Troubleshooting

### Боты не определяются

```caddyfile
bot_redirect {
    enable_debug true
    log_all_requests true
    verbose_metrics true
}
```

Проверьте логи:
```bash
journalctl -u caddy -f | grep bot_redirect
```

### Высокая нагрузка DNS

```caddyfile
bot_redirect {
    enable_reverse_dns false  # Временно отключить
    cache_ttl 4h             # Увеличить кеш
    max_dns_per_second 5     # Снизить лимит
}
```

### Медленная работа

```caddyfile
bot_redirect {
    cache_ttl 2h
    dns_timeout 3s
    max_cache_size 20000
    dns_worker_pool_size 10
}
```

### Проверка метрик

```bash
# Детальная статистика
curl -s http://localhost:2019/debug/vars | jq '.["bot_redirect.*"]'

# Основные метрики
curl http://localhost:2019/metrics
```

## 📚 Дополнительные примеры

### E-commerce с агрессивной защитой

```caddyfile
shop.example.com {
    bot_redirect {
        redirect_url https://promo.shop.example.com
        enable_reverse_dns true
        enable_referrer_check true
        enable_rate_limit true
        max_requests_per_ip 30
        max_dns_per_second 3
        cache_ttl 6h
        
        # Кастомная страница для прямых заходов
        empty_page_template `
            <!DOCTYPE html>
            <html>
            <head>
                <title>Shop Maintenance</title>
                <meta name="robots" content="noindex, nofollow">
            </head>
            <body>
                <h1>Site Under Maintenance</h1>
                <p>We'll be back soon!</p>
            </body>
            </html>
        `
    }
}
```

### Development с полным логированием

```caddyfile
dev.example.com {
    bot_redirect {
        redirect_url https://landing.dev.example.com
        enable_debug true
        log_all_requests true
        log_dns_queries true
        log_cache_ops true
        verbose_metrics true
        log_level debug
    }
}
```

### Высоконагруженный сайт

```caddyfile
high-traffic.example.com {
    bot_redirect {
        redirect_url https://landing.high-traffic.example.com
        
        # Оптимизированные настройки
        cache_ttl 8h
        max_cache_size 50000
        cleanup_interval 30m
        dns_timeout 2s
        max_dns_per_second 20
        dns_worker_pool_size 15
        
        # Отключаем медленные проверки
        enable_reverse_dns false
        enable_debug false
    }
}
```

## ⭐ Заключение

Плагин **полностью готов к использованию в production** и предоставляет enterprise-уровень функциональности для разделения трафика ботов и пользователей.

### Ключевые преимущества

✅ **Высокая производительность** - оптимизированные алгоритмы и кеширование  
✅ **Надежность** - graceful degradation и обработка ошибок  
✅ **Масштабируемость** - асинхронная обработка и worker pools  
✅ **Безопасность** - многоуровневая защита от обхода  
✅ **Мониторинг** - детальные метрики и логирование  
✅ **Гибкость** - множество настроек и кастомизации  

---

**Поддержка**: [GitHub Issues](https://github.com/your-username/caddy-bot-redirect/issues)  
**Документация**: [Wiki](https://github.com/your-username/caddy-bot-redirect/wiki)  
**Лицензия**: MIT