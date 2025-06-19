package botredirect

import (
	"bytes"
	"html/template"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Templates управляет HTML шаблонами для пустых страниц
type Templates struct {
	// Шаблоны
	emptyPageTemplate *template.Template
	customTemplate    string
	
	// Конфигурация
	enableCustom bool
	
	// Компоненты
	logger *zap.Logger
}

// TemplateData данные для шаблонов
type TemplateData struct {
	Title       string
	Message     string
	StatusCode  int
	Timestamp   time.Time
	UserAgent   string
	RemoteAddr  string
	RequestURI  string
	ServerName  string
}

// NewTemplates создает новый экземпляр системы шаблонов
func NewTemplates(config *Config, logger *zap.Logger) *Templates {
	t := &Templates{
		customTemplate: config.EmptyPageTemplate,
		enableCustom:   config.EmptyPageTemplate != "",
		logger:         logger,
	}

	// Инициализируем шаблоны
	if err := t.initializeTemplates(); err != nil {
		logger.Error("failed to initialize templates", zap.Error(err))
		// Fallback к базовому шаблону
		t.enableCustom = false
		t.initializeDefaultTemplate()
	}

	logger.Info("templates system initialized",
		zap.Bool("custom_template", t.enableCustom),
	)

	return t
}

// initializeTemplates инициализирует все шаблоны
func (t *Templates) initializeTemplates() error {
	if t.enableCustom {
		return t.initializeCustomTemplate()
	}
	return t.initializeDefaultTemplate()
}

// initializeDefaultTemplate инициализирует дефолтный шаблон
func (t *Templates) initializeDefaultTemplate() error {
	defaultTemplate := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{.Title}}</title>
    <meta name="robots" content="noindex, nofollow, noarchive, nosnippet">
    <meta name="googlebot" content="noindex, nofollow">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Arial, sans-serif;
            background-color: #f8f9fa;
            color: #333;
            text-align: center;
            margin: 0;
            padding: 50px 20px;
            line-height: 1.6;
        }
        .container {
            max-width: 600px;
            margin: 0 auto;
            background: white;
            padding: 40px;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        h1 {
            color: #6c757d;
            font-size: 2.5em;
            margin-bottom: 20px;
            font-weight: 300;
        }
        p {
            color: #6c757d;
            font-size: 1.1em;
            margin-bottom: 15px;
        }
        .error-code {
            font-size: 6em;
            font-weight: 100;
            color: #dee2e6;
            margin: 20px 0;
        }
        .footer {
            margin-top: 40px;
            padding-top: 20px;
            border-top: 1px solid #e9ecef;
            font-size: 0.9em;
            color: #adb5bd;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="error-code">{{.StatusCode}}</div>
        <h1>{{.Title}}</h1>
        <p>{{.Message}}</p>
        <div class="footer">
            <p>If you believe this is an error, please contact the website administrator.</p>
        </div>
    </div>
</body>
</html>`

	tmpl, err := template.New("empty_page").Parse(defaultTemplate)
	if err != nil {
		return err
	}

	t.emptyPageTemplate = tmpl
	return nil
}

// initializeCustomTemplate инициализирует кастомный шаблон
func (t *Templates) initializeCustomTemplate() error {
	tmpl, err := template.New("custom_empty_page").Parse(t.customTemplate)
	if err != nil {
		return err
	}

	t.emptyPageTemplate = tmpl
	return nil
}

// ServeEmptyPage отображает пустую страницу
func (t *Templates) ServeEmptyPage(w http.ResponseWriter, r *http.Request) error {
	// Подготавливаем данные для шаблона
	data := &TemplateData{
		Title:      "Page Not Found",
		Message:    "The requested page could not be found.",
		StatusCode: 404,
		Timestamp:  time.Now(),
		UserAgent:  r.UserAgent(),
		RemoteAddr: r.RemoteAddr,
		RequestURI: r.RequestURI,
		ServerName: r.Host,
	}

	// Устанавливаем заголовки
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive, nosnippet")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	
	// Устанавливаем статус код
	w.WriteHeader(http.StatusOK) // Возвращаем 200 чтобы не вызывать подозрений

	// Рендерим шаблон
	return t.emptyPageTemplate.Execute(w, data)
}

// ServeCustomPage отображает кастомную страницу с заданными параметрами
func (t *Templates) ServeCustomPage(w http.ResponseWriter, r *http.Request, title, message string, statusCode int) error {
	data := &TemplateData{
		Title:      title,
		Message:    message,
		StatusCode: statusCode,
		Timestamp:  time.Now(),
		UserAgent:  r.UserAgent(),
		RemoteAddr: r.RemoteAddr,
		RequestURI: r.RequestURI,
		ServerName: r.Host,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive, nosnippet")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	
	w.WriteHeader(statusCode)

	return t.emptyPageTemplate.Execute(w, data)
}

// RenderToString рендерит шаблон в строку (для тестирования)
func (t *Templates) RenderToString(data *TemplateData) (string, error) {
	var buf bytes.Buffer
	err := t.emptyPageTemplate.Execute(&buf, data)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// UpdateCustomTemplate обновляет кастомный шаблон в runtime
func (t *Templates) UpdateCustomTemplate(newTemplate string) error {
	tmpl, err := template.New("updated_custom_template").Parse(newTemplate)
	if err != nil {
		return err
	}

	t.emptyPageTemplate = tmpl
	t.customTemplate = newTemplate
	t.enableCustom = true

	t.logger.Info("custom template updated")
	return nil
}

// ResetToDefault сбрасывает к дефолтному шаблону
func (t *Templates) ResetToDefault() error {
	t.enableCustom = false
	t.customTemplate = ""
	
	err := t.initializeDefaultTemplate()
	if err != nil {
		return err
	}

	t.logger.Info("reset to default template")
	return nil
}

// ValidateTemplate проверяет корректность шаблона
func (t *Templates) ValidateTemplate(templateStr string) error {
	_, err := template.New("validation").Parse(templateStr)
	return err
}

// GetTemplateInfo возвращает информацию о текущем шаблоне
func (t *Templates) GetTemplateInfo() map[string]interface{} {
	return map[string]interface{}{
		"custom_enabled": t.enableCustom,
		"has_template":   t.emptyPageTemplate != nil,
		"template_size":  len(t.customTemplate),
	}
}