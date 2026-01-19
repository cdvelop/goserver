package server

// event: create,write,remove,rename
func (h *ServerHandler) NewFileEvent(fileName, extension, filePath, event string) error {
	h.strategyMu.RLock()
	strategy := h.strategy
	h.strategyMu.RUnlock()
	return strategy.HandleFileEvent(fileName, extension, filePath, event)
}
