package server

// Store defines the minimal interface for persistent storage
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// UI defines the minimal interface for UI interaction
type UI interface {
	RefreshUI()
}

const StoreKeyExternalServer = "server_external_mode"

type ServerModeHandler struct {
	h   *ServerHandler
	db  Store
	ui  UI
	log func(message ...any)
}

func NewServerModeHandler(h *ServerHandler, db Store, ui UI) *ServerModeHandler {
	return &ServerModeHandler{
		h:  h,
		db: db,
		ui: ui,
	}
}

func (s *ServerModeHandler) SetLog(f func(message ...any)) {
	s.log = f
}

func (s *ServerModeHandler) Name() string {
	return "ServerMode"
}

func (s *ServerModeHandler) Label() string {
	external := false
	if val, err := s.db.Get(StoreKeyExternalServer); err == nil && val == "true" {
		external = true
	}

	if external {
		return "SERVER MODE: EXTERNAL"
	}
	return "SERVER MODE: INTERNAL"
}

func (s *ServerModeHandler) Execute() {
	external := "false"
	if val, err := s.db.Get(StoreKeyExternalServer); err == nil && val == "true" {
		external = "true"
	}

	// Toggle
	if external == "true" {
		external = "false"
	} else {
		external = "true"
	}

	s.db.Set(StoreKeyExternalServer, external)

	// Update handler
	isExternal := (external == "true")
	if err := s.h.SetExternalServerMode(isExternal); err != nil {
		if s.log != nil {
			s.log("Error switching server mode:", err)
		}
		return
	}

	if s.log != nil {
		if isExternal {
			s.log("Switched to External Execution Mode")
		} else {
			s.log("Switched to Internal Execution Mode")
		}
	}

	if s.ui != nil {
		s.ui.RefreshUI()
	}
}
