package logging

import (
	"context"
	"log/slog"
)

// Attribute keys this package sets itself. They are constants so a call site
// cannot shadow one with a differently-spelled version of the same field, which
// is what makes a log file greppable.
const (
	// ComponentKey names the subsystem a record came from — "manifest",
	// "install", "roster".
	ComponentKey = "component"

	// CommandKey names the command the user ran — "init", "install", "lint".
	CommandKey = "command"
)

// contextKey is the key type for this package's context values. An unexported
// struct type cannot collide with a key from any other package.
type contextKey struct{ name string }

var (
	componentContextKey = contextKey{name: ComponentKey}
	commandContextKey   = contextKey{name: CommandKey}
)

// WithComponent labels every record made from ctx with the subsystem producing
// it. Components are set as work moves inward, so a record from the manifest
// loader carries both the command the user ran and the loader that failed.
func WithComponent(ctx context.Context, component string) context.Context {
	return context.WithValue(ctx, componentContextKey, component)
}

// WithCommand labels every record made from ctx with the command the user ran.
// It is set once, where the command begins, so nothing downstream has to thread
// the name through its own signatures to make a log entry attributable.
func WithCommand(ctx context.Context, command string) context.Context {
	return context.WithValue(ctx, commandContextKey, command)
}

// contextAttrs returns the attributes ctx carries, command before component:
// the outer label first, so a run reads from the outside in.
func contextAttrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}

	var attrs []slog.Attr
	if v, ok := ctx.Value(commandContextKey).(string); ok && v != "" {
		attrs = append(attrs, slog.String(CommandKey, v))
	}
	if v, ok := ctx.Value(componentContextKey).(string); ok && v != "" {
		attrs = append(attrs, slog.String(ComponentKey, v))
	}
	return attrs
}
