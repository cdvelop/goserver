package server

import "github.com/tinywasm/fmt"

func (h *ServerHandler) Name() string {
	return "SERVER"
}

// Label implements HandlerEdit.Label
func (h *ServerHandler) Label() string {
	return "Server Modes"
}

// Value implements HandlerEdit.Value
func (h *ServerHandler) Value() string {
	exec := "F"
	if !h.executionInternal {
		exec = "T"
	}
	return "Execution External:" + exec
}

// Change implements HandlerEdit.Change
func (h *ServerHandler) Change(newValue string) {
	pairs := fmt.Convert(newValue).Split(",")

	for _, pair := range pairs {
		// e.g., pair = "Execution External:T" or " Build OnDisk:F"
		s := fmt.Convert(pair).TrimSpace().String()
		pos := fmt.Index(s, ":")
		if pos == -1 {
			continue
		}

		key := fmt.Convert(s[:pos]).TrimSpace().String()
		val := fmt.Convert(s[pos+1:]).TrimSpace().ToLower().String()
		isTrue := val == "t" || val == "true"

		switch key {
		case "Execution External":
			if err := h.SetExternalServerMode(isTrue); err != nil {
				h.log("Mode change error:", err)
			}
		}
	}

	h.RefreshUI()
}

func (h *ServerHandler) RefreshUI() {
	if h.UI != nil {
		h.UI.RefreshUI()
	}
}
