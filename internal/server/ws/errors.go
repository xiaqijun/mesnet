package ws

import "errors"

var (
	ErrAgentOffline = errors.New("agent offline")
	ErrTimeout      = errors.New("command timeout")
)
