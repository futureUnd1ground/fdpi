// Package config defines the only user-controlled inputs accepted by the engine.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

const ApplicationVersion = "0.4.1"

type Strategy struct {
	Protocols   []string `json:"protocols"`
	Conditions  string   `json:"conditions"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TCP         string   `json:"tcp"`
	Split       string   `json:"split"`
	Fooling     string   `json:"fooling"`
	TTL         int      `json:"ttl"`
	Repeats     int      `json:"repeats"`
	QUIC        bool     `json:"quic"`
	HostCase    bool     `json:"host_case"`
	Risks       string   `json:"risks"`
}
type Profile struct {
	ID       string   `json:"id"`
	Name     string   `json:"name,omitempty"`
	Enabled  bool     `json:"enabled"`
	Strategy string   `json:"strategy"`
	Domains  []string `json:"domains"`
}
type Config struct {
	Version    int        `json:"version"`
	Strategies []Strategy `json:"strategies"`
	Profiles   []Profile  `json:"profiles"`
}

var idRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
var splitRE = regexp.MustCompile(`^(-?[0-9]{1,5}|(method|host|endhost|sld|endsld|midsld|sniext)([+-][0-9]{1,4})?)(,(-?[0-9]{1,5}|(method|host|endhost|sld|endsld|midsld|sniext)([+-][0-9]{1,4})?))*$`)
var labelRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func Domain(s string) bool {
	if len(s) > 253 || !strings.Contains(s, ".") || s != strings.ToLower(s) {
		return false
	}
	for _, l := range strings.Split(s, ".") {
		if !labelRE.MatchString(l) {
			return false
		}
	}
	return true
}
func Default() Config {
	s := []Strategy{
		{ID: "light", Name: "Light", Description: "TCP segmentation at byte 1", TCP: "multisplit", Split: "1", Repeats: 1},
		{ID: "balanced", Name: "Balanced", Description: "Fake plus split at SNI/HTTP host", TCP: "fake,multisplit", Split: "method+2,midsld", Fooling: "badseq", Repeats: 1, Risks: "Some middleboxes reject sequence manipulation"},
		{ID: "aggressive", Name: "Aggressive", Description: "Fake plus reverse segmentation, QUIC fake", TCP: "fake,multidisorder", Split: "1,midsld", Fooling: "badseq,md5sig", Repeats: 6, QUIC: true, Risks: "Extra packets; MD5 may be rejected differently by servers"},
		{ID: "tcp-split", Name: "TCP Split", Description: "Split original data into multiple TCP segments", TCP: "multisplit", Split: "1,5", Repeats: 1},
		{ID: "tls-focus", Name: "TLS Focus", Description: "Split TLS ClientHello at the SNI domain midpoint", TCP: "multisplit", Split: "midsld", Repeats: 1},
		{ID: "quic-focus", Name: "QUIC Focus", Description: "QUIC Initial fake; TCP unchanged", Repeats: 3, QUIC: true, Risks: "UDP 443 only; not a voice/media relay"},
		{ID: "ttl-fake", Name: "TTL Fake", Description: "Fixed TTL fake followed by segmentation", TCP: "fake,multisplit", Split: "1", TTL: 3, Repeats: 1, Risks: "TTL must reach DPI but not the server; route dependent"},
		{ID: "spotify-desktop", Name: "Spotify Desktop", Description: "TLS fake plus segmentation for Spotify desktop endpoints", TCP: "fake,multisplit", Split: "1,midsld", Fooling: "badseq", Repeats: 2, Risks: "HTTP/TLS only; native AP traffic on any port, geo and account restrictions are outside this profile"},
		{ID: "http-host", Name: "HTTP Host", Description: "Change HTTP Host header case", HostCase: true, Repeats: 1, Risks: "HTTP only; HTTPS remains unchanged"},
	}
	for i := range s {
		s[i].Conditions = "New local TCP handshakes on 80/443 with recognizable HTTP Host or TLS SNI"
		if s[i].TCP != "" {
			s[i].Protocols = []string{"http", "tls"}
		}
		if s[i].HostCase {
			s[i].Protocols = []string{"http"}
			s[i].Conditions = "Plain HTTP Host on TCP/80; encrypted HTTPS headers cannot be changed"
		}
		if s[i].QUIC {
			s[i].Protocols = append(s[i].Protocols, "quic")
			if s[i].TCP == "" {
				s[i].Conditions = "Recognizable QUIC Initial on UDP/443 with a matching hostname"
			}
		}
		if s[i].TTL > 0 {
			s[i].Conditions += "; calibrate TTL against the actual DPI/server route"
		}
		if s[i].Risks == "" {
			s[i].Risks = "Extra TCP segments; middlebox compatibility depends on the network"
		}
	}

	names := []string{"youtube", "steam", "discord"}
	domains := [][]string{{"youtube.com", "youtu.be", "googlevideo.com", "ytimg.com", "youtubei.googleapis.com"}, {"steampowered.com", "steamcommunity.com", "steamstatic.com"}, {"discord.com", "discord.gg", "discordapp.com", "discordapp.net", "discord.media"}}

	c := Config{Version: 1, Strategies: s}
	for i, n := range names {
		c.Profiles = append(c.Profiles, Profile{ID: n, Strategy: "balanced", Domains: domains[i]})
	}
	c.Profiles = append(c.Profiles, SpotifyProfile())
	LocalizeBuiltin(&c)
	return c
}
func (c Config) Strategy(id string) (Strategy, bool) {
	for _, s := range c.Strategies {
		if s.ID == id {
			return s, true
		}
	}
	return Strategy{}, false
}
func (c Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("неподдерживаемая версия конфигурации: %d", c.Version)
	}
	if len(c.Strategies) > 64 || len(c.Profiles) > 64 {
		return fmt.Errorf("допускается не более 64 стратегий и 64 профилей")
	}
	ids := map[string]bool{}
	for _, s := range c.Strategies {
		if !idRE.MatchString(s.ID) || ids[s.ID] {
			return fmt.Errorf("некорректный или повторяющийся ID стратегии: %q", s.ID)
		}
		ids[s.ID] = true
		if len(s.Name) > 80 || len(s.Description) > 512 || len(s.Risks) > 512 || len(s.Conditions) > 512 || len(s.Protocols) > 3 {
			return fmt.Errorf("слишком длинное описание стратегии или слишком много протоколов")
		}
		for _, p := range s.Protocols {
			switch p {
			case "http", "tls", "quic":
			default:
				return fmt.Errorf("неизвестный протокол в описании: %q", p)
			}
		}
		switch s.TCP {
		case "", "multisplit", "multidisorder", "fake", "fake,multisplit", "fake,multidisorder", "fakedsplit", "fakeddisorder":
		default:
			return fmt.Errorf("неподдерживаемый режим TCP: %q", s.TCP)
		}
		if s.Split != "" && (len(s.Split) > 128 || !splitRE.MatchString(s.Split)) {
			return fmt.Errorf("неверные позиции разделения пакетов")
		}
		if s.Fooling != "" {
			for _, f := range strings.Split(s.Fooling, ",") {
				switch f {
				case "badseq", "md5sig", "badsum", "ts":
				default:
					return fmt.Errorf("неподдерживаемый приём маскировки: %q", f)
				}
			}
		}
		if s.TTL < 0 || s.TTL > 255 || s.Repeats < 1 || s.Repeats > 10 {
			return fmt.Errorf("TTL должен быть от 0 до 255, число повторов — от 1 до 10")
		}
		if s.TCP == "" && !s.QUIC && !s.HostCase {
			return fmt.Errorf("стратегия должна содержать хотя бы один приём обработки")
		}
	}
	ids = map[string]bool{}
	domains := map[string]string{}
	for _, p := range c.Profiles {
		if !idRE.MatchString(p.ID) || ids[p.ID] {
			return fmt.Errorf("некорректный или повторяющийся ID профиля: %q", p.ID)
		}
		ids[p.ID] = true
		if len([]rune(p.Name)) > 60 || strings.IndexFunc(p.Name, unicode.IsControl) >= 0 {
			return fmt.Errorf("некорректное название сервиса")
		}
		if _, ok := c.Strategy(p.Strategy); !ok {
			return fmt.Errorf("неизвестная стратегия: %q", p.Strategy)
		}
		if len(p.Domains) > 10000 {
			return fmt.Errorf("в профиле допускается не более 10 000 доменов")
		}
		seen := map[string]bool{}
		for _, d := range p.Domains {
			if !Domain(d) || seen[d] {
				return fmt.Errorf("некорректный или повторяющийся домен: %q", d)
			}
			seen[d] = true
			if p.Enabled {
				if owner, ok := domains[d]; ok && owner != p.ID {
					return fmt.Errorf("домен пересекается в профилях %s и %s", owner, p.ID)
				}
				domains[d] = p.ID
			}
		}
		if p.Enabled && len(p.Domains) == 0 {
			return fmt.Errorf("у включённого профиля %s нет доменов", p.ID)
		}
	}
	for d, owner := range domains {
		parts := strings.Split(d, ".")
		for i := 1; i < len(parts)-1; i++ {
			if other, ok := domains[strings.Join(parts[i:], ".")]; ok && owner != other {
				return fmt.Errorf("домены пересекаются в профилях %s и %s", owner, other)
			}
		}
	}
	return nil
}
func Parse(b []byte) (Config, error) {
	var c Config
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	if d.Decode(new(any)) != io.EOF {
		return c, fmt.Errorf("после JSON обнаружены лишние данные")
	}
	return c, c.Validate()
}
func Load(path string) (Config, error) {
	f, e := os.Open(path)
	if e != nil {
		return Config{}, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 4<<20+1))
	if e != nil {
		return Config{}, e
	}
	if len(b) > 4<<20 {
		return Config{}, fmt.Errorf("размер конфигурации превышает 4 МиБ")
	}
	return Parse(b)
}
func Save(path string, c Config) error {
	if e := c.Validate(); e != nil {
		return e
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	return Write(path, append(b, '\n'), 0600)
}
func Write(path string, b []byte, mode os.FileMode) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".fdpi-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	dir, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func UserPath() string {
	d, e := os.UserConfigDir()
	if e != nil {
		d = "."
	}
	return filepath.Join(d, "fdpi", "config.json")
}

// Title returns a user-facing name while IDs remain stable for CLI and presets.
func (p Profile) Title() string {
	if p.Name != "" {
		return p.Name
	}
	switch p.ID {
	case "youtube":
		return "YouTube"
	case "spotify":
		return "Spotify Desktop"
	case "steam":
		return "Steam"
	case "discord":
		return "Discord"
	case "twitch":
		return "Twitch"
	case "telegram":
		return "Telegram"
	case "github":
		return "GitHub"
	case "custom":
		return "Свои сайты"
	}
	return p.ID
}

// SpotifyProfile covers first-party endpoints and narrowly named audio CDN hosts.
// Never match the whole shared akamaized.net zone.
func SpotifyProfile() Profile {
	return Profile{ID: "spotify", Name: "Spotify Desktop", Strategy: "spotify-desktop", Domains: []string{"spotify.com", "scdn.co", "spotifycdn.com", "spotifycdn.net", "audio-ak-spotify-com.akamaized.net", "audio4-ak-spotify-com.akamaized.net", "audio-akp-spotify-com.akamaized.net", "audio4-akp-spotify-com.akamaized.net"}}
}
