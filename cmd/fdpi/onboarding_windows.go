package main

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/futureUnd1ground/fdpi/internal/service"
)

// needsOnboarding показывает мастер первого запуска, пока служба не установлена
// или системные настройки отсутствуют.
func needsOnboarding() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st := service.State(ctx)
	_, e := os.Stat(service.SystemConfig)
	return !st.Installed || errors.Is(e, os.ErrNotExist)
}
