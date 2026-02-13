# Base 5: Graceful Handoff Protocol

## Sequence Diagram

```
devwatch detects modules/users/wasm/*.go changed
    → WasiBuilder.HandleFileEvent()
    → TinyGo recompiles → users.wasm (new)
    → server.HandleFileEvent() [if wasiStrategy]
    → wasiStrategy.swapModule("users", newWasmBytes)
        → load new instance (not yet active)
        → call old_instance.Drain(ctx, 5s):
            loop:
                ms := old_instance.drainFn.Call()
                if ms == 0: break
                if elapsed > 5s: log.Warn("drain timeout, forcing swap"); break
                sleep(time.Duration(ms) * time.Millisecond)
        → bus.Unsubscribe(all subscriptions for "users")
        → old_instance.Close()
        → new_instance.initFn.Call()
        → re-register new_instance subscriptions
        → swap in modules map
```

## Drain Timeout Config

```go
// In Config:
WasiDrainTimeout time.Duration // default: 5s if zero
```

## Error Cases
- **Module never returns 0**: force swap after timeout, emit warning log
- **New module fails to init()**: keep old module running, log error (no downtime)
- **wazero compilation error**: keep old module, surface error in TUI
- **Module panics after load**: recover via wazero context, mark module as failed, keep old
