# internal/

Reserved for code that genuinely needs to be shared across binaries, or that
must be extracted to break a Go import cycle. Everything else belongs in the
flat `cmd/harnaas/cli` package.

Adding a package here because it "feels like infrastructure" is not a reason.
The extraction trigger is objective: the compiler refuses the import.
