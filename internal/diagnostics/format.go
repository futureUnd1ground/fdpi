package diagnostics

import (
	"fmt"
	"strings"
)

// ResultText explains a probe without confusing HTTP reachability with DPI bypass.
func ResultText(r Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Домен: %s\nЭтап: %s\n", r.Target, stageText(r.Stage))
	if len(r.DNS) > 0 {
		fmt.Fprintf(&b, "Адреса DNS: %s\n", strings.Join(r.DNS, ", "))
	}
	if r.HTTP != 0 {
		fmt.Fprintf(&b, "Ответ HTTP: %d\n", r.HTTP)
	}
	fmt.Fprintf(&b, "Общее время: %d мс\n", r.Millis)
	if r.Error != "" {
		fmt.Fprintf(&b, "Ошибка: %s\n", r.Error)
	} else if r.Reachable {
		b.WriteString("HTTPS-ответ получен с проверкой сертификата.\n")
		if r.HTTP >= 400 {
			b.WriteString("Сайт ответил ошибкой; это не успешный доступ к приложению.\n")
		}
	}
	b.WriteString("Проверка не подтверждает работу видео, звонков или обход DPI.\n")
	return b.String()
}
func stageText(stage string) string {
	switch stage {
	case "dns":
		return "получение адреса DNS"
	case "tcp":
		return "подключение TCP"
	case "tls":
		return "TLS и проверка сертификата"
	case "http":
		return "запрос HTTPS"
	case "http-response":
		return "ответ HTTPS получен"
	}
	return stage
}

// ChecksText formats diagnostics for a terminal; exported JSON is unchanged.
func ChecksText(checks []Check) string {
	var b strings.Builder
	for _, c := range checks {
		mark := "✓"
		if !c.OK {
			mark = "!"
		}
		fmt.Fprintf(&b, "[%s] %s\n    %s\n", mark, c.Name, strings.ReplaceAll(strings.TrimSpace(c.Detail), "\n", "\n    "))
	}
	return b.String()
}

// ReportText formats measured strategy trials and the recommendation, if any.
func ReportText(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ПОДБОР СТРАТЕГИИ\nСервис: %s\n\n", report.Profile)
	for _, trial := range report.Trials {
		label := trial.Strategy
		if label == "baseline" {
			label = "без целевой стратегии"
		}
		verdict := "есть ошибки"
		if trial.Reachable {
			verdict = "все цели ответили с HTTP < 400"
		}
		fmt.Fprintf(&b, "%s — %s · %d мс\n", label, verdict, trial.Millis)
		if trial.YouTube != nil {
			b.WriteString(YouTubeText(*trial.YouTube))
		}
		for _, r := range trial.Results {
			for _, line := range strings.Split(strings.TrimSpace(ResultText(r)), "\n") {
				fmt.Fprintf(&b, "  %s\n", line)
			}
		}
		b.WriteByte('\n')
	}
	if report.Recommended != "" {
		fmt.Fprintf(&b, "Рекомендуемый ID: %s. Автоматически не применяется.\n", report.Recommended)
	} else {
		b.WriteString("Подтверждённой рекомендации нет.\n")
	}
	b.WriteString(report.Note + "\n")
	return b.String()
}
