package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/futureUnd1ground/fdpi/internal/service"
)

// runLogs читает файл журнала движка; --follow — поллинг новых строк.
// Фильтры --grep и --category работают как поиск по тексту записи.
func runLogs(ctx context.Context, args []string) error {
	follow := false
	grep := ""
	category := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--follow":
			follow = true
		case "--grep":
			i++
			if i >= len(args) {
				return errGrepArg()
			}
			grep = strings.ToLower(args[i])
		case "--category":
			i++
			if i >= len(args) {
				return errCategoryArg()
			}
			switch args[i] {
			case "err", "warning", "info":
				category = args[i]
			default:
				return errBadCategory()
			}
		default:
			return errUnknownLogArg()
		}
	}
	match := func(line string) bool {
		l := strings.ToLower(line)
		if grep != "" && !strings.Contains(l, grep) {
			return false
		}
		switch category {
		case "err":
			return strings.Contains(l, "err") || strings.Contains(l, "error") || strings.Contains(l, "fatal")
		case "warning":
			return strings.Contains(l, "warn")
		case "info":
			return true
		}
		return true
	}
	path := service.LogPath()
	if !follow {
		b, e := os.ReadFile(path)
		if e != nil {
			if os.IsNotExist(e) {
				fmt.Println("журнал пуст; служба ещё не запускалась")
				return nil
			}
			return e
		}
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		if len(lines) > 200 {
			lines = lines[len(lines)-200:]
		}
		for _, l := range lines {
			if match(l) {
				fmt.Println(l)
			}
		}
		return nil
	}
	// --follow: поллинг файла раз в 500 мс до отмены контекста (Ctrl+C).
	var offset int64
	for {
		st, e := os.Stat(path)
		if e == nil && st.Size() > offset {
			f, fe := os.Open(path)
			if fe == nil {
				_, _ = f.Seek(offset, 0)
				sc := bufio.NewScanner(f)
				sc.Buffer(make([]byte, 64*1024), 1024*1024)
				for sc.Scan() {
					if match(sc.Text()) {
						fmt.Println(sc.Text())
					}
				}
				offset, _ = f.Seek(0, 1)
				f.Close()
			}
		} else if e != nil && !os.IsNotExist(e) {
			return e
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
	}
}
