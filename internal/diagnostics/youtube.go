package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type EndpointResult struct {
	Name  string `json:"name"`
	HTTP  int    `json:"http"`
	Bytes int    `json:"bytes"`
	Local string `json:"local_address,omitempty"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}
type YouTubeReport struct {
	Interface string           `json:"interface,omitempty"`
	Checks    []EndpointResult `json:"checks"`
	OK        bool             `json:"ok"`
	Note      string           `json:"note"`
}

var clientVersion = regexp.MustCompile(`"INNERTUBE_CLIENT_VERSION"\s*:\s*"([0-9.]+)"`)

// YouTube verifies page data, a thumbnail and the anonymous home API independently.
// It neither reads browser cookies nor treats this as video playback evidence.
func YouTube(ctx context.Context, iface string) YouTubeReport {
	return YouTubeResolved(ctx, iface, nil)
}

// YouTubeResolved pins only test endpoints while retaining TLS hostname validation.
func YouTubeResolved(ctx context.Context, iface string, addresses map[string]string) YouTubeReport {
	for host, ip := range addresses {
		if (host != "www.youtube.com" && host != "i.ytimg.com") || net.ParseIP(ip) == nil {
			return YouTubeReport{Checks: []EndpointResult{{Name: "Адрес проверки", Error: "Допустимы только IP для www.youtube.com и i.ytimg.com"}}}
		}
	}

	d := net.Dialer{Timeout: 5 * time.Second}
	if iface != "" {
		if _, err := net.InterfaceByName(iface); err != nil {
			return YouTubeReport{Interface: iface, Checks: []EndpointResult{{Name: "Интерфейс", Error: err.Error()}}}
		}
		d.Control = func(_, _ string, c syscall.RawConn) error {
			var err error
			e := c.Control(func(fd uintptr) { err = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, iface) })
			if e != nil {
				return e
			}
			return err
		}
	}
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if ip := addresses[host]; ip != "" {
			address = net.JoinHostPort(ip, port)
		}
		return d.DialContext(ctx, network, address)
	}
	tr := &http.Transport{DialContext: dial, ForceAttemptHTTP2: true, DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	report := youtube(ctx, client, "https://www.youtube.com", "https://i.ytimg.com")
	report.Interface = iface
	return report
}
func youtube(ctx context.Context, client *http.Client, site, images string) YouTubeReport {
	report := YouTubeReport{Note: "Проверены анонимная главная, API и изображение. Воспроизведение видео, QUIC и персональная лента не проверены. Без --interface запросы могут идти через VPN. DNS использует системный резолвер."}
	request := func(name, method, url string, body []byte) ([]byte, EndpointResult) {
		r := EndpointResult{Name: name}
		ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(i httptrace.GotConnInfo) { r.Local = i.Conn.LocalAddr().String() }})
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			r.Error = err.Error()
			return nil, r
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 FDPI-diagnostics")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			r.Error = err.Error()
			return nil, r
		}
		defer resp.Body.Close()
		r.HTTP = resp.StatusCode
		data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
		r.Bytes = len(data)
		if err != nil {
			r.Error = err.Error()
		} else if len(data) > 4<<20 {
			r.Error = "Ответ превышает лимит 4 МиБ"
		} else if r.HTTP != 200 {
			r.Error = fmt.Sprintf("HTTP %d", r.HTTP)
		} else {
			r.OK = true
		}
		return data, r
	}
	page, r := request("Главная YouTube", http.MethodGet, site+"/", nil)
	version := clientVersion.FindSubmatch(page)
	if r.OK && len(version) != 2 {
		r.OK = false
		r.Error = "В странице нет версии клиента YouTube; возможны заглушка или страница согласия"
	}
	report.Checks = append(report.Checks, r)
	image, r := request("Изображение YouTube", http.MethodGet, images+"/vi/dQw4w9WgXcQ/hqdefault.jpg", nil)
	if r.OK && !validJPEG(image) {
		r.OK = false
		r.Error = "Вместо JPEG получены другие данные"
	}
	report.Checks = append(report.Checks, r)
	if len(version) == 2 {
		payload, _ := json.Marshal(map[string]any{"context": map[string]any{"client": map[string]string{"clientName": "WEB", "clientVersion": string(version[1]), "hl": "ru"}}, "browseId": "FEwhat_to_watch"})
		data, api := request("API главной YouTube", http.MethodPost, site+"/youtubei/v1/browse?prettyPrint=false", payload)
		if api.OK {
			var parsed map[string]json.RawMessage
			err := json.Unmarshal(data, &parsed)
			var contents map[string]json.RawMessage
			contentErr := json.Unmarshal(parsed["contents"], &contents)
			if err != nil || parsed["error"] != nil || contentErr != nil || len(contents) == 0 {
				api.OK = false
				api.Error = "API не вернул JSON с содержимым главной"
			}
		}
		report.Checks = append(report.Checks, api)
	} else {
		report.Checks = append(report.Checks, EndpointResult{Name: "API главной YouTube", Error: "Пропущен: версия клиента не получена из страницы"})
	}
	report.OK = true
	for _, c := range report.Checks {
		report.OK = report.OK && c.OK
	}
	return report
}
func YouTubeText(r YouTubeReport) string {
	var b strings.Builder
	for _, c := range r.Checks {
		mark := "ОШИБКА"
		if c.OK {
			mark = "OK"
		}
		fmt.Fprintf(&b, "[%s] %s: HTTP %d, %d байт, адрес %s\n", mark, c.Name, c.HTTP, c.Bytes, c.Local)
		if c.Error != "" {
			fmt.Fprintln(&b, "  "+c.Error)
		}
	}
	fmt.Fprintln(&b, r.Note)
	return b.String()
}

func validJPEG(data []byte) bool {
	c, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || c.Width <= 0 || c.Height <= 0 || c.Width > 4096 || c.Height > 4096 {
		return false
	}
	_, err = jpeg.Decode(bytes.NewReader(data))
	return err == nil
}
