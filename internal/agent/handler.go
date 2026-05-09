package agent

import "encoding/json"

// Handler dispatches incoming commands from the control plane.
type Handler struct {
	agent *Agent
}

func NewHandler(agent *Agent) *Handler {
	return &Handler{agent: agent}
}

func (h *Handler) Handle(msg map[string]json.RawMessage) {
	var id, action string
	json.Unmarshal(msg["id"], &id)
	json.Unmarshal(msg["action"], &action)

	args := msg["args"]

	result, err := h.agent.HandleCommand(action, args)

	// Send result back via management WS
	resp := map[string]any{
		"type": "result",
		"id":   id,
	}

	if err != nil {
		resp["status"] = "error"
		resp["error"] = err.Error()
	} else {
		resp["status"] = "ok"
		if result != nil {
			resp["data"] = result
		}
	}

	h.agent.ws.SendJSON(resp)
}
