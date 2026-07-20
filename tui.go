package server

const (
	LabelServerRun = "Server Run"
)

func (h *ServerHandler) Name() string {
	return "SERVER"
}

// Label implements HandlerSelection.Label
func (h *ServerHandler) Label() string {
	return "Server Run"
}

// Value implements HandlerSelection.Value
func (h *ServerHandler) Value() string {
	h.strategyMu.RLock()
	isInternal := h.executionInternal
	h.strategyMu.RUnlock()

	if isInternal {
		return execModeInternal
	}
	return execModeExternal
}

// Options implements HandlerSelection.Options
func (h *ServerHandler) Options() []map[string]string {
	return []map[string]string{
		{execModeInternal: "Internal"},
		{execModeExternal: "External"},
	}
}

// Change implements HandlerSelection.Change
func (h *ServerHandler) Change(newValue string) {
	switch newValue {
	case execModeInternal:
		if err := h.SetExternalServerMode(false); err != nil {
			h.log("Mode change error:", err)
		}
	case execModeExternal:
		if err := h.SetExternalServerMode(true); err != nil {
			h.log("Mode change error:", err)
		}
	default:
		h.log("Error: Unknown execution mode:", newValue)
	}

	h.RefreshUI()
}

// SetLog implements devtui.Loggable
func (h *ServerHandler) SetLog(fn func(...any)) {
	h.SetLogger(fn)
}

func (h *ServerHandler) RefreshUI() {
	if h.UI != nil {
		h.UI.RefreshUI()
	}
}
