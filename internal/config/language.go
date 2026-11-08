package config

// LocalizeBuiltin translates known stock metadata, preserving IDs, parameters and custom text.
// This also migrates drafts created by FDPI versions with English descriptions.
func LocalizeBuiltin(c *Config) bool {
	changed := false
	for i := range c.Strategies {
		s := &c.Strategies[i]
		for _, field := range []*string{&s.Name, &s.Description, &s.Risks, &s.Conditions} {
			if translated, ok := russianMetadata[*field]; ok {
				*field = translated
				changed = true
			}
		}
	}
	return changed
}

var russianMetadata = map[string]string{
	"TLS fake plus segmentation for Spotify desktop endpoints":                                            "Ложный TLS-пакет и разделение для Spotify Desktop",
	"HTTP/TLS only; native AP traffic on any port, geo and account restrictions are outside this profile": "Только HTTP/TLS на 80/443. Нативный AP-трафик, регион и ограничения аккаунта не обходятся",
	"Light":                            "Лёгкая",
	"Balanced":                         "Сбалансированная",
	"Aggressive":                       "Агрессивная",
	"TCP Split":                        "Разделение TCP",
	"TLS Focus":                        "Обработка TLS",
	"QUIC Focus":                       "Обработка QUIC",
	"TTL Fake":                         "Подмена с TTL",
	"HTTP Host":                        "Заголовок Host",
	"TCP segmentation at byte 1":       "Разделение TCP после первого байта",
	"Fake plus split at SNI/HTTP host": "Ложный пакет и разделение по домену SNI/HTTP Host",
	"Fake plus reverse segmentation, QUIC fake":                                 "Ложные пакеты, обратный порядок TCP-сегментов и обработка QUIC",
	"Split original data into multiple TCP segments":                            "Разделение исходных данных на несколько TCP-сегментов",
	"Split TLS ClientHello at the SNI domain midpoint":                          "Разделение TLS ClientHello в середине домена SNI",
	"QUIC Initial fake; TCP unchanged":                                          "Ложный QUIC Initial; обработка только UDP/443",
	"Fixed TTL fake followed by segmentation":                                   "Ложный пакет с заданным TTL, затем разделение TCP",
	"Change HTTP Host header case":                                              "Изменение регистра букв заголовка HTTP Host",
	"Some middleboxes reject sequence manipulation":                             "Некоторые сетевые устройства отвергают изменение номеров последовательности TCP",
	"Extra packets; MD5 may be rejected differently by servers":                 "Дополнительные пакеты; серверы по-разному обрабатывают опцию MD5",
	"UDP 443 only; not a voice/media relay":                                     "Только UDP/443; голос и медиапотоки не туннелируются",
	"TTL must reach DPI but not the server; route dependent":                    "TTL должен позволить пакету дойти до DPI, но не до сервера; зависит от маршрута",
	"HTTP only; HTTPS remains unchanged":                                        "Только HTTP; зашифрованный HTTPS не изменяется",
	"Extra TCP segments; middlebox compatibility depends on the network":        "Дополнительные TCP-сегменты; совместимость зависит от сетевого оборудования",
	"New local TCP handshakes on 80/443 with recognizable HTTP Host or TLS SNI": "Новые локальные TCP-соединения на портах 80/443 с распознаваемым HTTP Host или TLS SNI",
	"Plain HTTP Host on TCP/80; encrypted HTTPS headers cannot be changed":      "Открытый HTTP на TCP/80; зашифрованные заголовки HTTPS изменить нельзя",
	"Recognizable QUIC Initial on UDP/443 with a matching hostname":             "Распознаваемый QUIC Initial на UDP/443 с подходящим доменом",
	"New local TCP handshakes on 80/443 with recognizable HTTP Host or TLS SNI; calibrate TTL against the actual DPI/server route": "Новые TCP-соединения на 80/443 с HTTP Host или TLS SNI; TTL нужно подобрать под маршрут до DPI и сервера",
}

// TrimUnusedStock removes only untouched, disabled legacy examples.
// Custom domains, names, enabled profiles and assigned custom strategies survive.
func TrimUnusedStock(c *Config) bool {
	legacy := map[string][]string{"twitch": {"twitch.tv", "ttvnw.net", "jtvnw.net"}, "telegram": {"telegram.org", "t.me", "web.telegram.org"}, "github": {"github.com", "githubusercontent.com", "githubassets.com"}, "custom": {}}
	changed := false
	profiles := make([]Profile, 0, len(c.Profiles))
	for _, p := range c.Profiles {
		stock, ok := legacy[p.ID]
		same := ok && len(stock) == len(p.Domains)
		if same {
			for i := range stock {
				if stock[i] != p.Domains[i] {
					same = false
					break
				}
			}
		}
		if same && !p.Enabled && p.Name == "" && p.Strategy == "balanced" {
			changed = true
			continue
		}
		profiles = append(profiles, p)
	}
	if changed {
		c.Profiles = profiles
	}
	return changed
}
