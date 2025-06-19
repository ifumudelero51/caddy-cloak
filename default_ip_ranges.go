package botredirect

// getExtendedBotIPRanges возвращает расширенный список IP диапазонов ботов
// Организован по компаниям и сервисам для лучшего понимания
func getExtendedBotIPRanges() []string {
	return []string{
		// === GOOGLE ===
		// Основные диапазоны Googlebot
		"66.249.64.0/19",    // Основной диапазон Googlebot
		"66.249.64.0/20",    // Подсеть 1
		"66.249.80.0/20",    // Подсеть 2
		"66.249.96.0/19",    // Расширенный диапазон
		
		// Google Cloud и сервисы
		"64.233.160.0/19",   // Google services
		"64.233.192.0/18",   // Google infrastructure
		"72.14.192.0/18",    // Google Apps
		"74.125.0.0/16",     // Gmail, Google services
		"108.177.8.0/21",    // Google Cloud
		"173.194.0.0/16",    // YouTube, Google services
		"209.85.128.0/17",   // Google AdSense
		"216.239.32.0/19",   // Google Search
		"172.217.0.0/16",    // Google services
		"142.250.0.0/15",    // Google infrastructure
		"172.253.0.0/16",    // Google Cloud Platform
		"34.64.0.0/10",      // Google Cloud Asia
		"35.184.0.0/13",     // Google Cloud US
		"104.154.0.0/15",    // Google Compute Engine
		"130.211.0.0/16",    // Google Cloud Load Balancer
		"35.235.240.0/20",   // Google Cloud Functions
		
		// Google IPv6
		"2001:4860::/32",    // Google IPv6 primary
		"2404:6800::/32",    // Google IPv6 Asia
		"2607:f8b0::/32",    // Google IPv6 Americas
		"2800:3f0::/32",     // Google IPv6 South America
		"2a00:1450::/32",    // Google IPv6 Europe
		"2c0f:fb50::/32",    // Google IPv6 Africa
		
		// === MICROSOFT / BING ===
		// Bing crawler ranges
		"40.77.167.0/24",    // Bingbot primary
		"157.55.39.0/24",    // Bingbot secondary
		"207.46.13.0/24",    // MSN crawler
		"207.46.204.0/24",   // MSN services
		"65.52.0.0/14",      // Microsoft services
		"131.253.0.0/16",    // Microsoft infrastructure
		"157.54.0.0/15",     // Microsoft Azure
		"157.56.0.0/14",     // Microsoft services
		"168.61.0.0/16",     // Azure US East
		"168.62.0.0/15",     // Azure US West
		"191.232.0.0/13",    // Azure Brazil
		"104.40.0.0/13",     // Azure global
		"13.64.0.0/11",      // Azure new ranges
		"20.0.0.0/8",        // Azure expanded
		"40.64.0.0/10",      // Azure US
		"52.224.0.0/11",     // Azure services
		
		// Microsoft Office 365
		"132.245.0.0/16",    // Office 365
		"134.170.0.0/16",    // Exchange Online
		"157.58.0.0/15",     // SharePoint Online
		"199.30.16.0/20",    // Office 365 services
		"204.79.197.0/24",   // Bing services
		
		// === YANDEX ===
		// Yandex main ranges
		"5.45.192.0/18",     // YandexBot primary
		"5.255.192.0/18",    // Yandex services
		"37.9.64.0/18",      // Yandex infrastructure
		"37.140.128.0/18",   // Yandex CDN
		"77.88.0.0/16",      // Yandex main
		"84.201.128.0/18",   // Yandex Cloud
		"87.250.224.0/19",   // Yandex services
		"93.158.128.0/18",   // Yandex infrastructure
		"95.108.128.0/17",   // Yandex expanded
		"178.154.128.0/17",  // Yandex global
		"199.21.96.0/22",    // Yandex CDN US
		"213.180.192.0/19",  // Yandex Europe
		
		// === BAIDU ===
		// Baidu spider ranges
		"119.63.192.0/21",   // Baiduspider
		"180.76.0.0/16",     // Baidu crawler
		"220.181.32.0/19",   // Baidu services
		"123.125.64.0/18",   // Baidu infrastructure
		"61.135.128.0/17",   // Baidu Beijing
		"202.108.16.0/20",   // Baidu services
		"14.215.128.0/17",   // Baidu Guangzhou
		"103.235.224.0/19",  // Baidu global
		
		// === FACEBOOK / META ===
		// Facebook crawler ranges
		"31.13.24.0/21",     // Facebook primary
		"31.13.64.0/18",     // Facebook services
		"66.220.144.0/20",   // Facebook infrastructure
		"69.63.176.0/20",    // Facebook apps
		"69.171.224.0/19",   // Facebook CDN
		"74.119.76.0/22",    // Facebook services
		"103.4.96.0/22",     // Facebook Asia
		"173.252.64.0/18",   // Facebook global
		"204.15.20.0/22",    // Facebook infrastructure
		"157.240.0.0/16",    // Facebook expanded
		"179.60.192.0/22",   // Facebook Latin America
		"185.60.216.0/22",   // Facebook Europe
		"129.134.0.0/16",    // Facebook services
		
		// Instagram
		"54.230.0.0/16",     // Instagram CDN
		"52.84.0.0/15",      // Instagram services
		
		// WhatsApp
		"158.85.0.0/16",     // WhatsApp services
		"75.126.0.0/16",     // WhatsApp infrastructure
		
		// === TWITTER / X ===
		// Twitter bot ranges
		"199.16.156.0/22",   // Twitter services
		"199.59.148.0/22",   // Twitter API
		"192.133.76.0/22",   // Twitter infrastructure
		"104.244.42.0/21",   // Twitter CDN
		"69.195.160.0/19",   // Twitter legacy
		"185.45.5.0/24",     // Twitter Europe
		"202.160.128.0/22",  // Twitter Asia
		
		// === LINKEDIN ===
		// LinkedIn bot ranges
		"108.174.0.0/16",    // LinkedIn services
		"144.2.0.0/16",      // LinkedIn infrastructure
		"65.156.0.0/16",     // LinkedIn legacy
		"216.52.242.0/24",   // LinkedIn crawler
		
		// === OTHER SOCIAL MEDIA ===
		// Pinterest
		"54.236.0.0/15",     // Pinterest services
		"107.178.192.0/18",  // Pinterest infrastructure
		
		// Reddit
		"151.101.0.0/16",    // Reddit CDN
		"198.41.208.0/21",   // Reddit services
		
		// TikTok
		"161.117.0.0/16",    // TikTok global
		"203.107.0.0/16",    // TikTok Asia
		
		// === SEARCH ENGINES ===
		// DuckDuckGo
		"20.191.45.212",     // DuckDuckGo bot
		"40.88.21.235",      // DuckDuckGo services
		"52.142.26.175",     // DuckDuckGo infrastructure
		
		// Yahoo (Verizon Media)
		"68.180.224.0/19",   // Yahoo crawler
		"72.30.0.0/16",      // Yahoo services
		"98.136.0.0/14",     // Yahoo infrastructure
		"206.190.32.0/19",   // Yahoo legacy
		
		// Ask.com
		"65.214.44.0/22",    // Ask Jeeves
		"208.185.109.0/24",  // Ask services
		
		// === SEO TOOLS ===
		// Ahrefs
		"54.36.148.0/24",    // Ahrefs bot
		"54.36.149.0/24",    // Ahrefs services
		"195.154.0.0/16",    // Ahrefs infrastructure
		"163.172.0.0/16",    // Ahrefs Europe
		
		// SEMrush
		"185.191.171.0/24",  // SEMrush bot
		"87.236.176.0/20",   // SEMrush services
		"46.229.168.0/22",   // SEMrush infrastructure
		
		// Moz
		"108.171.248.0/21",  // Moz crawler
		"64.4.0.0/18",       // Moz services
		
		// Majestic
		"109.74.192.0/20",   // Majestic SEO
		"185.227.68.0/22",   // Majestic infrastructure
		
		// === MONITORING SERVICES ===
		// Pingdom
		"85.195.116.0/22",   // Pingdom EU
		"173.248.128.0/17",  // Pingdom US
		"174.34.224.0/20",   // Pingdom global
		"184.75.208.0/20",   // Pingdom services
		"50.23.94.0/24",     // Pingdom monitoring
		"95.211.0.0/16",     // Pingdom infrastructure
		"64.237.48.0/20",    // Pingdom legacy
		"208.64.24.0/21",    // Pingdom expanded
		
		// UptimeRobot
		"69.162.124.0/24",   // UptimeRobot
		"63.143.42.0/24",    // UptimeRobot services
		"50.30.204.0/24",    // UptimeRobot monitoring
		"46.137.190.0/24",   // UptimeRobot EU
		"122.248.234.0/24",  // UptimeRobot Asia
		
		// StatusCake
		"185.232.130.0/24",  // StatusCake
		"178.255.215.0/24",  // StatusCake EU
		"198.245.49.0/24",   // StatusCake US
		
		// Site24x7
		"104.236.0.0/16",    // Site24x7 US
		"138.197.0.0/16",    // Site24x7 services
		"159.65.0.0/16",     // Site24x7 global
		
		// === SECURITY SCANNERS ===
		// Qualys
		"64.39.96.0/20",     // Qualys scanner
		"208.64.32.0/20",    // Qualys services
		"67.231.145.0/24",   // Qualys infrastructure
		
		// Rapid7
		"71.6.128.0/17",     // Rapid7 scanner
		"208.118.227.0/24",  // Rapid7 services
		
		// Shodan
		"198.20.87.98",      // Shodan scanner
		"209.126.110.38",    // Shodan crawler
		"93.120.27.62",      // Shodan EU
		"71.6.165.200",      // Shodan US
		"82.221.105.6",      // Shodan global
		"71.6.167.142",      // Shodan services
		"66.240.192.138",    // Shodan infrastructure
		"194.28.115.245",    // Shodan monitoring
		
		// Censys
		"192.35.168.0/23",   // Censys scanner
		"162.142.125.0/24",  // Censys services
		
		// === CLOUD PROVIDERS ===
		// Amazon Web Services (AWS)
		"52.0.0.0/11",       // AWS US East
		"54.0.0.0/8",        // AWS global
		"3.0.0.0/8",         // AWS expanded
		"18.0.0.0/8",        // AWS services
		"34.192.0.0/10",     // AWS infrastructure
		"52.192.0.0/11",     // AWS Asia Pacific
		"52.28.0.0/16",      // AWS Europe
		"52.64.0.0/12",      // AWS Australia
		
		// Cloudflare
		"103.21.244.0/22",   // Cloudflare IPv4
		"103.22.200.0/22",   // Cloudflare services
		"103.31.4.0/22",     // Cloudflare global
		"104.16.0.0/13",     // Cloudflare CDN
		"108.162.192.0/18",  // Cloudflare infrastructure
		"131.0.72.0/22",     // Cloudflare expanded
		"141.101.64.0/18",   // Cloudflare services
		"162.158.0.0/15",    // Cloudflare global
		"172.64.0.0/13",     // Cloudflare enterprise
		"173.245.48.0/20",   // Cloudflare legacy
		"188.114.96.0/20",   // Cloudflare EU
		"190.93.240.0/20",   // Cloudflare Latin America
		"197.234.240.0/22",  // Cloudflare Africa
		"198.41.128.0/17",   // Cloudflare US
		
		// Cloudflare IPv6
		"2400:cb00::/32",    // Cloudflare IPv6 primary
		"2606:4700::/32",    // Cloudflare IPv6 secondary
		"2803:f800::/32",    // Cloudflare IPv6 Latin America
		"2405:b500::/32",    // Cloudflare IPv6 Asia
		"2405:8100::/32",    // Cloudflare IPv6 Australia
		"2a06:98c0::/29",    // Cloudflare IPv6 Europe
		"2c0f:f248::/32",    // Cloudflare IPv6 Africa
		
		// DigitalOcean
		"159.65.0.0/16",     // DigitalOcean
		"128.199.0.0/16",    // DigitalOcean Asia
		"46.101.0.0/16",     // DigitalOcean EU
		"138.197.0.0/16",    // DigitalOcean US
		"104.236.0.0/16",    // DigitalOcean legacy
		"167.99.0.0/16",     // DigitalOcean expanded
		"178.62.0.0/16",     // DigitalOcean Europe
		"188.166.0.0/16",    // DigitalOcean global
		"206.189.0.0/16",    // DigitalOcean new
		"165.22.0.0/16",     // DigitalOcean services
		
		// Linode
		"173.255.0.0/16",    // Linode US
		"96.126.0.0/18",     // Linode services
		"50.116.0.0/18",     // Linode infrastructure
		"69.164.192.0/18",   // Linode legacy
		"109.74.192.0/20",   // Linode EU
		"139.162.0.0/16",    // Linode global
		"45.79.0.0/16",      // Linode expanded
		"172.104.0.0/15",    // Linode new
		"23.239.0.0/17",     // Linode services
		
		// === ACADEMIC / RESEARCH ===
		// Internet Archive
		"207.241.224.0/20",  // Internet Archive
		"209.131.36.158",    // Wayback Machine
		"208.70.24.0/21",    // Archive.org
		
		// Common Crawl
		"54.236.0.0/15",     // Common Crawl AWS
		"52.0.0.0/11",       // Common Crawl infrastructure
		
		// === CONTENT DELIVERY NETWORKS ===
		// Fastly
		"23.235.32.0/20",    // Fastly global
		"43.249.72.0/22",    // Fastly Asia
		"103.244.50.0/24",   // Fastly services
		"103.245.222.0/23",  // Fastly infrastructure
		"146.75.0.0/16",     // Fastly US
		"151.101.0.0/16",    // Fastly CDN
		"157.52.64.0/18",    // Fastly Europe
		"167.82.0.0/17",     // Fastly expanded
		"185.31.16.0/22",    // Fastly EU
		"199.27.72.0/21",    // Fastly services
		
		// KeyCDN
		"103.254.154.0/24",  // KeyCDN
		"114.129.2.0/24",    // KeyCDN Asia
		"177.67.208.0/22",   // KeyCDN Latin America
		
		// MaxCDN (StackPath)
		"94.46.143.0/24",    // MaxCDN
		"118.214.128.0/17",  // MaxCDN Asia
		"177.54.144.0/20",   // MaxCDN Brazil
		
		// === OTHER CRAWLERS ===
		// Archive Team
		"208.70.24.0/21",    // Archive Team
		"207.241.224.0/20",  // Archive services
		
		// Webrecorder
		"35.237.130.0/24",   // Webrecorder
		"35.184.0.0/13",     // Webrecorder services
		
		// === SPECIFIC BOT IPs ===
		// Various known bot IPs that don't fit into ranges
		"5.9.12.248",        // Generic crawler
		"46.229.168.139",    // SEMrush bot
		"54.36.148.92",      // Ahrefs bot
		"85.17.26.180",      // Generic bot
		"94.102.49.190",     // European crawler
		"95.108.213.30",     // Yandex bot
		"108.162.216.204",   // Cloudflare bot
		"149.56.227.104",    // OVH bot
		"163.172.65.55",     // Scaleway bot
		"178.162.202.30",    // Generic crawler
		"185.93.3.123",      // European bot
		"192.99.4.36",       // OVH crawler
		"207.154.244.106",   // DigitalOcean bot
		
		// === IPv6 RANGES ===
		// Major IPv6 bot ranges
		"2001:67c:4e8::/48", // European crawlers
		"2a01:4f8::/29",     // Hetzner (many bots)
		"2a01:4f9::/32",     // Hetzner expanded
		"2001:41d0::/32",    // OVH (many crawlers)
		"2604:a880::/32",    // DigitalOcean IPv6
		"2400:6180::/32",    // DigitalOcean Asia IPv6
		"2a03:b0c0::/32",    // DigitalOcean EU IPv6
	}
}

// getBotRangesByOrganization возвращает диапазоны, сгруппированные по организациям
func getBotRangesByOrganization() map[string][]string {
	return map[string][]string{
		"Google": {
			"66.249.64.0/19", "64.233.160.0/19", "72.14.192.0/18",
			"74.125.0.0/16", "108.177.8.0/21", "173.194.0.0/16",
			"209.85.128.0/17", "216.239.32.0/19", "172.217.0.0/16",
			"142.250.0.0/15", "172.253.0.0/16", "2001:4860::/32",
		},
		"Microsoft": {
			"40.77.167.0/24", "157.55.39.0/24", "207.46.13.0/24",
			"65.52.0.0/14", "131.253.0.0/16", "157.54.0.0/15",
			"13.64.0.0/11", "20.0.0.0/8", "40.64.0.0/10",
		},
		"Yandex": {
			"5.45.192.0/18", "77.88.0.0/16", "95.108.128.0/17",
			"178.154.128.0/17", "87.250.224.0/19", "93.158.128.0/18",
		},
		"Facebook": {
			"31.13.24.0/21", "31.13.64.0/18", "173.252.64.0/18",
			"157.240.0.0/16", "69.171.224.0/19", "204.15.20.0/22",
		},
		"Baidu": {
			"119.63.192.0/21", "180.76.0.0/16", "220.181.32.0/19",
			"123.125.64.0/18", "61.135.128.0/17", "202.108.16.0/20",
		},
		"Cloudflare": {
			"103.21.244.0/22", "104.16.0.0/13", "108.162.192.0/18",
			"162.158.0.0/15", "172.64.0.0/13", "173.245.48.0/20",
		},
		"Amazon": {
			"52.0.0.0/11", "54.0.0.0/8", "3.0.0.0/8", "18.0.0.0/8",
			"34.192.0.0/10", "52.192.0.0/11",
		},
	}
}

// getHighConfidenceBotRanges возвращает диапазоны с высокой степенью уверенности
func getHighConfidenceBotRanges() []string {
	return []string{
		// Только основные диапазоны поисковых систем
		"66.249.64.0/19",    // Google
		"40.77.167.0/24",    // Bing
		"5.45.192.0/18",     // Yandex
		"119.63.192.0/21",   // Baidu
		"180.76.0.0/16",     // Baidu
		"2001:4860::/32",    // Google IPv6
	}
}

// getBotRangesByRegion возвращает диапазоны по географическим регионам
func getBotRangesByRegion() map[string][]string {
	return map[string][]string{
		"North America": {
			"66.249.64.0/19", "64.233.160.0/19", "40.77.167.0/24",
			"173.252.64.0/18", "199.16.156.0/22", "108.174.0.0/16",
		},
		"Europe": {
			"85.195.116.0/22", "185.232.130.0/24", "94.46.143.0/24",
			"185.31.16.0/22", "46.229.168.0/22", "195.154.0.0/16",
		},
		"Asia": {
			"119.63.192.0/21", "180.76.0.0/16", "220.181.32.0/19",
			"161.117.0.0/16", "203.107.0.0/16", "114.129.2.0/24",
		},
		"Russia": {
			"5.45.192.0/18", "77.88.0.0/16", "95.108.128.0/17",
			"178.154.128.0/17", "87.250.224.0/19", "93.158.128.0/18",
		},
	}
}

// getDefaultBotIPRanges возвращает базовый список IP диапазонов ботов
func getDefaultBotIPRanges() []string {
    return []string{
        // Google
        "66.249.64.0/19",
        "64.233.160.0/19", 
        "72.14.192.0/18",
        "74.125.0.0/16",
        
        // Microsoft Bing
        "40.77.167.0/24",
        "157.55.39.0/24",
        "207.46.13.0/24",
        
        // Yandex
        "5.45.192.0/18",
        "77.88.0.0/16",
        "95.108.128.0/17",
        
        // Facebook
        "31.13.24.0/21",
        "173.252.64.0/18",
        
        // Базовые диапазоны
        "199.16.156.0/22", // Twitter
        "185.232.130.0/24", // StatusCake
        "85.195.116.0/22", // Pingdom
    }
}

// getDefaultAllowedReferrers возвращает базовый список разрешенных referrer доменов
func getDefaultAllowedReferrers() []string {
    return []string{
        // Google
        "google.com", "*.google.com", "google.ru", "*.google.ru",
        "google.de", "*.google.de", "google.fr", "*.google.fr",
        
        // Bing
        "bing.com", "*.bing.com", "msn.com", "*.msn.com",
        
        // Yandex
        "yandex.ru", "*.yandex.ru", "yandex.com", "*.yandex.com",
        "ya.ru", "*.ya.ru",
        
        // Другие поисковики
        "duckduckgo.com", "*.duckduckgo.com",
        "yahoo.com", "*.yahoo.com", "search.yahoo.com",
        "baidu.com", "*.baidu.com",
        "sogou.com", "*.sogou.com",
        "so.com", "*.so.com",
        "ask.com", "*.ask.com",
        "aol.com", "*.aol.com",
        "ecosia.org", "*.ecosia.org",
        "startpage.com", "*.startpage.com",
    }
}

// getDefaultBotUserAgents возвращает базовый список User-Agent паттернов ботов
func getDefaultBotUserAgents() []string {
    return getBasicBotUserAgents() // Используем уже определенную функцию
}