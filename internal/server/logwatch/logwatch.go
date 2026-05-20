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

var global = &Buffer{cap: 2000, entries: make([]Entry, 0, 2000)}

func Debug(source, msg string) { global.add("DEBUG", source, msg) }
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

func GetLogs(since time.Time, level, source string, limit int) []Entry {
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
		if source != "" && e.Source != source {
			continue
		}
		result = append(result, e)
		if limit > 0 && len(result) >= limit {
			break
		}
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
