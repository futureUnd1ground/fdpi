package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/futureUnd1ground/fdpi/internal/config"
)

func onboardingPath() string {
	return filepath.Join(filepath.Dir(config.UserPath()), "onboarding.json")
}

// needsOnboarding показывает мастер первого запуска, пока он не пройден
// для текущей версии.
func needsOnboarding() bool {
	b, e := os.ReadFile(onboardingPath())
	if e == nil {
		var saved struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(b, &saved) == nil && saved.Version == version {
			return false
		}
	}
	return true
}
