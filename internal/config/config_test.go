package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

func TestDefaultRoundTrip(t *testing.T) {
	c := Default()
	for i := range c.Profiles {
		if len(c.Profiles[i].Domains) > 0 {
			c.Profiles[i].Enabled = true
		}
	}
	p := filepath.Join(t.TempDir(), "config.json")
	if e := Save(p, c); e != nil {
		t.Fatal(e)
	}
	d, e := Load(p)
	if e != nil || len(d.Strategies) != 9 {
		t.Fatalf("%v %+v", e, d)
	}
}
func TestRejectUntrustedInputs(t *testing.T) {
	for _, mutate := range []func(*Config){func(c *Config) { c.Profiles[0].ID = "../evil" }, func(c *Config) { c.Strategies[0].TCP = "fake;reboot" }, func(c *Config) { c.Strategies[0].Split = "1 --user=root" }, func(c *Config) { c.Profiles[0].Domains = []string{"https://evil.com"} }, func(c *Config) { c.Strategies[0].Repeats = 999 }, func(c *Config) { c.Profiles[0].Strategy = "missing" }, func(c *Config) {
		c.Profiles[0].Enabled = true
		c.Profiles[1].Enabled = true
		c.Profiles[1].Domains = []string{"sub." + c.Profiles[0].Domains[0]}
	}} {
		c := Default()
		mutate(&c)
		if c.Validate() == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if _, e := Parse([]byte(`{"version":1,"exec":"reboot"}`)); e == nil {
		t.Fatal("unknown field accepted")
	}
}
func TestLargeHostlistAndOverlap(t *testing.T) {
	c := Default()
	c.Profiles = c.Profiles[:1]
	c.Profiles[0].Enabled = true
	c.Profiles[0].Domains = nil
	for i := 0; i < 10000; i++ {
		c.Profiles[0].Domains = append(c.Profiles[0].Domains, fmt.Sprintf("host%d.example.org", i))
	}
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	c.Profiles = append(c.Profiles, Profile{ID: "other", Enabled: true, Strategy: "light", Domains: []string{"example.org"}})
	if c.Validate() == nil {
		t.Fatal("parent overlap accepted")
	}
}
func FuzzParse(f *testing.F) {
	b, _ := json.Marshal(Default())
	f.Add(b)
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 4<<20 {
			t.Skip()
		}
		c, e := Parse(b)
		if e == nil {
			if e = c.Validate(); e != nil {
				t.Fatal(e)
			}
		}
	})
}
func BenchmarkParse(b *testing.B) {
	v, _ := json.Marshal(Default())
	b.ReportAllocs()
	for b.Loop() {
		if _, e := Parse(v); e != nil {
			b.Fatal(e)
		}
	}
}
func BenchmarkHostlist10k(b *testing.B) {
	c := Default()
	c.Profiles = c.Profiles[:1]
	c.Profiles[0].Enabled = true
	c.Profiles[0].Domains = nil
	for i := 0; i < 10000; i++ {
		c.Profiles[0].Domains = append(c.Profiles[0].Domains, fmt.Sprintf("h%d.example.org", i))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if e := c.Validate(); e != nil {
			b.Fatal(e)
		}
	}
}

func TestStockServicesAndSafeMigration(t *testing.T) {
	c := Default()
	if len(c.Profiles) != 4 || c.Profiles[0].ID != "youtube" || c.Profiles[1].ID != "steam" || c.Profiles[2].ID != "discord" {
		t.Fatal(c.Profiles)
	}
	c.Profiles = append(c.Profiles, Profile{ID: "twitch", Strategy: "balanced", Domains: []string{"twitch.tv", "ttvnw.net", "jtvnw.net"}}, Profile{ID: "github", Name: "Мой GitHub", Strategy: "balanced", Domains: []string{"github.com"}}, Profile{ID: "my-site", Strategy: "light", Domains: []string{"example.org"}})
	if !TrimUnusedStock(&c) {
		t.Fatal("legacy stock not removed")
	}
	if len(c.Profiles) != 6 || c.Profiles[4].Name != "Мой GitHub" || c.Profiles[5].ID != "my-site" {
		t.Fatal("custom profile lost", c.Profiles)
	}
}
