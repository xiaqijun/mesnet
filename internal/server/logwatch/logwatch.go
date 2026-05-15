package logwatch

import (
	"sync"
	"time"
)

type Entry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Source  string    `json:"source"`
	Message string    `json:"message"`
}

type Buffer struct {
	entries []Entry
	mu      sync.RWMutex
	cap     int
}

var global = &Buffer{cap: 200, entries: make([]Entry, 0, 200)}

func Info(source, msg string)  { global.add("INFO", source, msg) }
func Error(source, msg string) { global.add("ERROR", source, msg) }
func Warn(source, msg string)  { global.add("WARN", source, msg) }

func (b *Buffer) add(level, source, msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.entries) >= b.cap {
		b.entries = b.entries[1:]
	}
	b.entries = append(b.entries, Entry{Time: time.Now(), Level: level, Source: source, Message: msg})
}

func GetLogs(since time.Time, level string) []Entry {
	global.mu.RLock()
	defer global.mu.RUnlock()
	var result []Entry
	for _, e := range global.entries {
		if e.Time.Before(since) {
			continue
		}
		if level != "" && e.Level != level {
			continue
		}
		result = append(result, e)
	}
	return result
}

func GetErrors() []Entry {
	global.mu.RLock()
	defer global.mu.RUnlock()
	var result []Entry
	for _, e := range global.entries {
		if e.Level == "ERROR" {
			result = append(result, e)
		}
	}
	return result
}
