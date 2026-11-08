// Package diagnostics separates transport reachability from evidence of DPI bypass.
package diagnostics

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/engine"
	"github.com/futureUnd1ground/fdpi/internal/network"
	"github.com/futureUnd1ground/fdpi/internal/service"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Result struct {
	Target    string    `json:"target"`
	Stage     string    `json:"stage"`
	DNS       []string  `json:"dns,omitempty"`
	HTTP      int       `json:"http,omitempty"`
	Millis    int64     `json:"milliseconds"`
	Reachable bool      `json:"reachable"`
	Error     string    `json:"error,omitempty"`
	At        time.Time `json:"at"`
}

func Probe(ctx context.Context, domain string) Result {
	d := net.Dialer{Timeout: 3 * time.Second}
	return probe(ctx, domain, net.DefaultResolver.LookupHost, d.DialContext, &tls.Config{MinVersion: tls.VersionTLS12})
}
func probe(ctx context.Context, domain string, lookup func(context.Context, string) ([]string, error), dial func(context.Context, string, string) (net.Conn, error), tlsConfig *tls.Config) Result {
	r := Result{Target: domain, Stage: "dns", At: time.Now().UTC()}
	start := time.Now()
	if !config.Domain(domain) {
		r.Error = "некорректный домен"
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	ips, e := lookup(ctx, domain)
	r.DNS = ips
	finish := func(err error) Result {
		r.Millis = time.Since(start).Milliseconds()
		if err != nil {
			r.Error = err.Error()
		}
		return r
	}
	if e != nil {
		return finish(e)
	}
	if len(ips) == 0 {
		return finish(fmt.Errorf("DNS не вернул адресов"))
	}
	r.Stage = "tcp"
	var conn net.Conn
	for _, ip := range ips {
		conn, e = dial(ctx, "tcp", net.JoinHostPort(ip, "443"))
		if e == nil {
			break
		}
	}
	if e != nil {
		return finish(e)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	r.Stage = "tls"
	tcfg := tlsConfig.Clone()
	tcfg.ServerName = domain
	tc := tls.Client(conn, tcfg)
	if e = tc.HandshakeContext(ctx); e != nil {
		return finish(e)
	}
	// Consume the already verified connection once, with redirects and proxy environment disabled.
	r.Stage = "http"
	used := false
	transport := &http.Transport{DisableKeepAlives: true, DialTLSContext: func(context.Context, string, string) (net.Conn, error) {
		if used {
			return nil, fmt.Errorf("проверочное соединение уже использовано")
		}
		used = true
		return tc, nil
	}}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(ctx, "GET", "https://"+domain+"/", nil)
	if e != nil {
		return finish(e)
	}
	req.Header.Set("User-Agent", "FDPI-diagnostics/0.1")
	resp, e := client.Do(req)
	if e != nil {
		return finish(e)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	r.HTTP = resp.StatusCode
	r.Reachable = true
	r.Stage = "http-response"
	return finish(nil)
}

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func Doctor(ctx context.Context) []Check {
	var out []Check
	add := func(n string, e error, d string) {
		if e != nil {
			d = e.Error()
		}
		out = append(out, Check{n, e == nil, d})
	}
	_, e := os.Stat(engine.Path)
	add("Файл движка", e, engine.Path)
	c, e := config.Load(service.SystemConfig)
	add("Системные настройки", e, service.SystemConfig)
	if e == nil {
		dir, e := os.MkdirTemp("", "fdpi-doctor-")
		if e == nil {
			defer os.RemoveAll(dir)
			a, _, _, ae := engine.Args(c, dir)
			if ae == nil {
				ae = engine.Check(ctx, engine.Path, a)
			}
			add("Совместимость движка", ae, "параметры приняты реальным движком через --dry-run")
		}
	}
	b, e := exec.CommandContext(ctx, "/usr/bin/nft", "--version").CombinedOutput()
	add("nftables", e, strings.TrimSpace(string(b)))
	_, e = network.Owned(ctx)
	add("Доступ к таблице и её принадлежность", e, "проверка таблицы без изменений выполнена")
	b, e = exec.CommandContext(ctx, "/usr/bin/nft", "-j", "list", "ruleset").CombinedOutput()
	if e == nil {
		text := string(b)
		if strings.Contains(text, "flowtable") || strings.Contains(text, "\"queue\"") {
			out = append(out, Check{"Возможные конфликты firewall", false, "найдена очередь или flowtable; проверьте nft list ruleset, включая правила самого FDPI"})
		} else {
			add("Возможные конфликты firewall", nil, "очереди и flowtable не найдены; другие взаимодействия с firewall всё же возможны")
		}
	}
	acct, ae := os.ReadFile("/proc/sys/net/netfilter/nf_conntrack_acct")
	if ae == nil && strings.TrimSpace(string(acct)) != "1" {
		ae = fmt.Errorf("нужно значение net.netfilter.nf_conntrack_acct=1; FDPI не меняет его автоматически")
	}
	add("Учёт соединений conntrack", ae, strings.TrimSpace(string(acct)))
	_, e = os.Stat("/proc/net/netfilter/nfnetlink_queue")
	add("Поддержка NFQUEUE в ядре", e, "доступна; может потребоваться загрузка модуля")
	b, e = os.ReadFile("/proc/self/status")
	add("Полномочия процесса в ядре", e, capLine(string(b)))
	add("Права администратора", func() error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("обычный пользователь — это нормально для интерфейса; административные действия используют sudo")
		}
		return nil
	}(), "root")
	_, e = net.DefaultResolver.LookupHost(ctx, "example.com")
	add("DNS", e, "адрес example.com получен")
	s := service.State(ctx)
	out = append(out, Check{"Служба systemd", s.Detail == "" && s.Installed, s.StateText()})
	return out
}
func capLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "CapEff:") {
			return l
		}
	}
	return "поле CapEff недоступно"
}

type Trial struct {
	Strategy  string         `json:"strategy"`
	YouTube   *YouTubeReport `json:"youtube,omitempty"`
	Results   []Result       `json:"results"`
	Millis    int64          `json:"milliseconds"`
	Reachable bool           `json:"reachable"`
}
type Report struct {
	Profile     string  `json:"profile"`
	Trials      []Trial `json:"trials"`
	Recommended string  `json:"recommended,omitempty"`
	Note        string  `json:"note"`
}

func Lab(ctx context.Context, id string) (report Report, err error) {
	report.Profile = id
	report.Note = "Проверяется доступность HTTPS. HTTP 403/451 может означать блокировку приложения. QUIC и медиатрафик не проверяются. Рекомендация появляется только при улучшении относительно неудачной базовой проверки."
	if os.Geteuid() != 0 {
		return report, fmt.Errorf("для подбора нужны права root и установленная служба systemd")
	}
	if err = engine.Trusted(service.SystemConfig); err != nil {
		return
	}
	if err = engine.Trusted(engine.Path); err != nil {
		return
	}
	lock, e := service.Lock("/etc/fdpi/.operation.lock")
	if e != nil {
		return report, e
	}
	defer service.Unlock(lock)
	original, e := config.Load(service.SystemConfig)
	if e != nil {
		return report, e
	}
	index := -1
	for i, p := range original.Profiles {
		if p.ID == id {
			index = i
		}
	}
	if index < 0 {
		return report, fmt.Errorf("неизвестный сервис: %s", id)
	}
	domains := original.Profiles[index].Domains
	if len(domains) == 0 {
		return report, fmt.Errorf("список доменов пуст")
	}
	if len(domains) > 2 {
		domains = domains[:2]
	}
	state := service.State(ctx)
	if state.Detail != "" || !state.Installed {
		return report, fmt.Errorf("служба systemd недоступна: %s", state.Detail)
	}
	wasActive := state.Service == "active"
	// Restore on cancellation and every error, using an independent context.
	defer func() {
		restore, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		e := config.Save(service.SystemConfig, original)
		action := "stop"
		if wasActive {
			action = "restart"
		}
		_, se := service.Command(restore, action, "fdpi.service")
		if e != nil || se != nil {
			err = fmt.Errorf("результат подбора: %v; ОШИБКА ВОССТАНОВЛЕНИЯ: настройки=%v служба=%v", err, e, se)
		}
	}()
	for _, sid := range []string{"baseline", "light", "balanced", "tcp-split", "tls-focus", "aggressive"} {
		select {
		case <-ctx.Done():
			return report, ctx.Err()
		default:
		}
		candidate := original
		candidate.Profiles = append([]config.Profile(nil), original.Profiles...)
		candidate.Profiles[index].Enabled = sid != "baseline"
		if sid != "baseline" {
			candidate.Profiles[index].Strategy = sid
		}
		if e = candidate.Validate(); e != nil {
			return report, e
		}
		if e = config.Save(service.SystemConfig, candidate); e != nil {
			return report, e
		}
		if e = service.RestartCheck(ctx); e != nil {
			return report, e
		}
		trial := Trial{Strategy: sid, Reachable: true}
		if id == "youtube" {
			start := time.Now()
			suite := YouTube(ctx, "")
			trial.YouTube = &suite
			trial.Reachable = suite.OK
			trial.Millis = time.Since(start).Milliseconds()
		} else {
			for _, d := range domains {
				r := Probe(ctx, d)
				trial.Results = append(trial.Results, r)
				trial.Reachable = trial.Reachable && r.Reachable && r.HTTP < 400
				trial.Millis += r.Millis
			}
		}
		report.Trials = append(report.Trials, trial)
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return report, ctx.Err()
		case <-timer.C:
		}
	}
	if !report.Trials[0].Reachable {
		best := int64(1 << 62)
		for _, t := range report.Trials[1:] {
			if t.Reachable && t.Millis < best {
				best = t.Millis
				report.Recommended = t.Strategy
			}
		}
	}
	return report, nil
}
