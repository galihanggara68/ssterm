// Package theme defines the look of a Screenshot.
package theme

// Theme is a pluggable look for a specific terminal.
type Theme struct {
	Name       string
	Foreground string // CSS color
	Background string // CSS color
}

// Default is the built-in dark Theme.
func Default() Theme {
	return Theme{Name: "dark", Foreground: "#c5c8c6", Background: "#1d1f21"}
}

// Lookup returns the Theme with the given name.
func Lookup(name string) (Theme, bool) {
	if name == "dark" {
		return Default(), true
	}
	return Theme{}, false
}
