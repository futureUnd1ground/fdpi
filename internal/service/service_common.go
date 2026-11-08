// Package service supervises the DPI engine and restores the last accepted configuration.
package service

import (
	"encoding/json"
	"errors"
	"github.com/futureUnd1ground/fdpi/internal/config"
)

// ErrConflict denotes a permanent conflict that restarting the daemon cannot fix.
var ErrConflict = errors.New("конфликт сетевых движков")

func commit(path, backup string, c config.Config, active bool, restart, restore func() error) error {
	old, e := config.Load(path)
	if e != nil {
		return e
	}
	if e = config.Save(backup, old); e != nil {
		return e
	}
	if e = config.Save(path, c); e != nil {
		return e
	}
	if active {
		if e = restart(); e != nil {
			return errors.Join(e, config.Save(path, old), restore())
		}
	}
	return nil
}

type Status struct {
	Installed       bool   `json:"installed"`
	QueueObservable bool   `json:"queue_observable"`
	Service         string `json:"service"`
	UnitFileState   string `json:"unit_file_state,omitempty"`
	SupervisorPID   int    `json:"supervisor_pid,omitempty"`
	QueueBound      bool   `json:"queue_bound"`
	Detail          string `json:"detail,omitempty"`
}

func JSON(v any) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }

func (s Status) QueueText() string {
	if !s.QueueObservable {
		return "неизвестно (нужны права root)"
	}
	if s.QueueBound {
		return "подключена"
	}
	return "не подключена"
}

// StateText renders service states for people; JSON keeps the stable identifiers.
func (s Status) StateText() string {
	switch s.Service {
	case "active":
		return "работает"
	case "inactive":
		return "остановлена"
	case "failed":
		return "ошибка запуска"
	case "activating":
		return "запускается"
	case "deactivating":
		return "останавливается"
	case "not-installed":
		return "не установлена — выполните fdpi setup"
	case "unknown":
		return "состояние неизвестно"
	}
	return s.Service
}
