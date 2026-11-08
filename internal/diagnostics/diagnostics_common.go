// Package diagnostics separates transport reachability from evidence of DPI bypass.
package diagnostics

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"github.com/futureUnd1ground/fdpi/internal/engine"
	"github.com/futureUnd1ground/fdpi/internal/service"
	"io"
	"net"
	"net/http"
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
	if e := requireRoot(); e != nil {
		return report, e
	}
	if err = engine.Trusted(service.SystemConfig); err != nil {
		return
	}
	if err = engine.Trusted(engine.Path); err != nil {
		return
	}
	lock, e := service.Lock(service.OperationLockPath())
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
		se := restoreService(restore, action)
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
