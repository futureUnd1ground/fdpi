package tui

import (
	"encoding/json"
	"fmt"
	"github.com/futureUnd1ground/fdpi/internal/config"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Palette struct{ Name, Background, Foreground, Accent string }

var Themes = []string{"FDPI", "Auto", "Catppuccin Mocha", "Catppuccin Latte", "Tokyo Night", "Gruvbox", "Nord", "Monochrome", "Terminal", "wallust", "pywal", "matugen"}
var hex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func contrast(a, b string) float64 {
	lum := func(s string) float64 {
		n, _ := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
		v := []float64{float64(n>>16) / 255, float64((n>>8)&255) / 255, float64(n&255) / 255}
		for i, x := range v {
			if x <= .04045 {
				v[i] = x / 12.92
			} else {
				v[i] = math.Pow((x+.055)/1.055, 2.4)
			}
		}
		return .2126*v[0] + .7152*v[1] + .0722*v[2]
	}
	x, y := lum(a), lum(b)
	if x < y {
		x, y = y, x
	}
	return (x + .05) / (y + .05)
}
func ReadPalette(path, name string) (Palette, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return Palette{}, e
	}
	if len(b) > 1<<20 {
		return Palette{}, fmt.Errorf("файл палитры слишком большой")
	}
	var v map[string]any
	if e = json.Unmarshal(b, &v); e != nil {
		return Palette{}, e
	}
	get := func(v any) string {
		switch x := v.(type) {
		case string:
			return x
		case map[string]any:
			if h, ok := x["hex"].(string); ok {
				return h
			}
		}
		return ""
	}
	m := v
	if special, ok := v["special"].(map[string]any); ok {
		m = special
	} else if colors, ok := v["colors"].(map[string]any); ok {
		m = colors
		if dark, ok := m["dark"].(map[string]any); ok {
			m = dark
		}
	}
	p := Palette{Name: name, Background: get(m["background"]), Foreground: get(m["foreground"]), Accent: get(m["primary"])}
	if p.Foreground == "" {
		p.Foreground = get(m["on_background"])
	}
	if p.Accent == "" {
		if colors, ok := v["colors"].(map[string]any); ok {
			p.Accent = get(colors["color4"])
		}
	}
	if !hex.MatchString(p.Background) || !hex.MatchString(p.Foreground) || !hex.MatchString(p.Accent) {
		return p, fmt.Errorf("нужны цвета #RRGGBB: background, foreground/on_background и primary/color4")
	}
	if contrast(p.Background, p.Foreground) < 4.5 {
		if contrast(p.Background, "#ffffff") > contrast(p.Background, "#000000") {
			p.Foreground = "#ffffff"
		} else {
			p.Foreground = "#000000"
		}
	}
	if contrast(p.Background, p.Accent) < 3 {
		p.Accent = p.Foreground
	}
	return p, nil
}
func Theme(name string) Palette {
	cache, e := os.UserCacheDir()
	if e != nil {
		cache = "."
	}
	conf, _ := os.UserConfigDir()
	if name == "Auto" || name == "wallust" || name == "pywal" || name == "matugen" {
		for _, src := range []struct{ name, path string }{{"wallust", filepath.Join(cache, "wallust", "colors.json")}, {"pywal", filepath.Join(cache, "wal", "colors.json")}, {"matugen", filepath.Join(conf, "fdpi", "matugen.json")}} {
			if name == "Auto" || name == src.name {
				if p, e := ReadPalette(src.path, src.name); e == nil {
					return p
				}
			}
		}
	}
	switch name {
	case "FDPI":
		return Palette{"FDPI", "#10111b", "#eeeef6", "#ad91ff"}
	case "Catppuccin Latte":
		return Palette{name, "#eff1f5", "#4c4f69", "#1e66f5"}
	case "Tokyo Night":
		return Palette{name, "#1a1b26", "#c0caf5", "#7aa2f7"}
	case "Gruvbox":
		return Palette{name, "#282828", "#ebdbb2", "#fabd2f"}
	case "Nord":
		return Palette{name, "#2e3440", "#eceff4", "#88c0d0"}
	case "Monochrome":
		return Palette{name, "#111111", "#eeeeee", "#ffffff"}
	case "Terminal":
		return Palette{name, "0", "7", "4"}
	}
	return Palette{"Catppuccin Mocha", "#1e1e2e", "#cdd6f4", "#89b4fa"}
}
func themePath() string { return filepath.Join(filepath.Dir(config.UserPath()), "theme.json") }
func LoadTheme() string {
	b, e := os.ReadFile(themePath())
	if e == nil {
		var s string
		if json.Unmarshal(b, &s) == nil {
			for _, n := range Themes {
				if s == n {
					return n
				}
			}
		}
	}
	return "FDPI"
}
func SaveTheme(n string) error { b, _ := json.Marshal(n); return config.Write(themePath(), b, 0600) }
