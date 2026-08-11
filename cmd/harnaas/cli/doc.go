// Package cli holds the harnaas command tree.
//
// The package is deliberately flat: command files are named
// <noun>_group.go for a group root and <noun>_<verb>.go for a leaf
// command, and a subpackage is extracted only when Go's import-cycle
// rule forces it. See CLAUDE.md for the conventions this layout carries.
package cli
