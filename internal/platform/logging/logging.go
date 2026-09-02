// Package logging владеет схемой лог-записи приложения: сборкой slog-обработчика,
// набором обязательных полей, реестром стабильных идентификаторов типов записей
// и редакцией секретов.
//
// Пакет живёт в платформенном слое и не зависит ни от http, ни от usecase, ни от
// store, поэтому им может пользоваться любой слой без протекания абстракций.
// Транспортным типом логгера остаётся стандартный *slog.Logger — собственный
// интерфейс поверх stdlib здесь не нужен.
//
// Конфигурация читается только в composition root (cmd/server) и передаётся сюда
// значениями: пакет сам переменные окружения не читает.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Ключи обязательных и контекстных полей записи. Это внешний контракт: на них
// строятся запросы и дашборды в Kibana, поэтому переименование ключа ломает
// потребителя так же, как переименование поля API.
const (
	KeyEvent     = "event"
	KeyService   = "service"
	KeyEnv       = "env"
	KeyRequestID = "request_id"
	KeyTenantID  = "tenant_id"
	KeyActorID   = "actor_id"
	KeyTeamID    = "team_id"
	KeyPeriodID  = "period_id"
)

// Format — формат вывода записи.
type Format string

const (
	// FormatJSON — машиночитаемый вывод для системы сбора логов. По умолчанию.
	FormatJSON Format = "json"
	// FormatText — человекочитаемый вывод для локальной разработки. Состав полей
	// от формата не зависит.
	FormatText Format = "text"
)

// Значения по умолчанию, применяемые при пустой или некорректной конфигурации.
const (
	DefaultLevel   = slog.LevelInfo
	DefaultFormat  = FormatJSON
	DefaultService = "okrs"
	DefaultEnv     = "dev"
)

// Config — конфигурация логгера. Строки приходят из окружения развёртывания,
// поэтому любое их значение должно быть безопасным: некорректное не мешает
// запуску, а приводит к значению по умолчанию и предупреждению в логе.
type Config struct {
	Level   string
	Format  string
	Service string
	Env     string
	// Output по умолчанию os.Stdout. Приложение не открывает соединений
	// с системой сбора логов — доставка является ответственностью инфраструктуры.
	Output io.Writer
}

// ParseLevel разбирает уровень логирования. Пустая строка — это не ошибка, а
// «настройка не задана»: применяется уровень по умолчанию.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return DefaultLevel, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return DefaultLevel, fmt.Errorf("logging: неизвестный уровень %q", s)
	}
}

// ParseFormat разбирает формат вывода. Пустая строка означает формат по умолчанию.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return DefaultFormat, nil
	case string(FormatJSON):
		return FormatJSON, nil
	case string(FormatText):
		return FormatText, nil
	default:
		return DefaultFormat, fmt.Errorf("logging: неизвестный формат %q", s)
	}
}

// New собирает логгер по конфигурации. Некорректные значения не препятствуют
// запуску: применяется значение по умолчанию, а о самой некорректности пишется
// предупреждение — иначе опечатка в манифесте развёртывания тихо меняла бы
// наблюдаемость приложения.
func New(cfg Config) *slog.Logger {
	var problems []string

	level, err := ParseLevel(cfg.Level)
	if err != nil {
		problems = append(problems, err.Error())
	}
	format, err := ParseFormat(cfg.Format)
	if err != nil {
		problems = append(problems, err.Error())
	}

	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	service := cfg.Service
	if service == "" {
		service = DefaultService
	}
	env := cfg.Env
	if env == "" {
		env = DefaultEnv
	}

	build := func(lvl slog.Leveler) *slog.Logger {
		opts := &slog.HandlerOptions{Level: lvl, ReplaceAttr: RedactAttr}
		var base slog.Handler
		if format == FormatText {
			base = slog.NewTextHandler(out, opts)
		} else {
			base = slog.NewJSONHandler(out, opts)
		}
		return slog.New(newContextHandler(base)).With(
			slog.String(KeyService, service),
			slog.String(KeyEnv, env),
		)
	}

	logger := build(level)

	if len(problems) > 0 {
		// Диагностика конфигурации выводится отдельным логгером, не подчинённым
		// настроенному порогу.
		//
		// Иначе она бесполезна ровно тогда, когда нужна: при LOG_LEVEL=error
		// предупреждение о некорректном LOG_FORMAT отфильтровывалось бы самим
		// уровнем, о соседе которого сообщает, и опечатка в манифесте тихо меняла
		// бы формат вывода. Уровень записи остаётся warn — сообщение об откате
		// к значению по умолчанию не является отказом.
		diag := build(slog.LevelWarn)
		for _, p := range problems {
			diag.Warn("некорректная настройка логирования, применено значение по умолчанию",
				slog.String(KeyEvent, EventConfigInvalid),
				slog.String("problem", p),
			)
		}
	}
	return logger
}

// Discard возвращает логгер, который ничего не пишет. Для тестов, которым логи
// не нужны.
func Discard() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}
