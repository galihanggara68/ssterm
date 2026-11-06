// Package theme defines the look of a Screenshot.
package theme

// Theme is a pluggable look for a specific terminal.
type Theme struct {
	Name       string
	Foreground string // CSS color
	Background string // CSS color
	// ANSI holds the 16 ANSI colors (CSS colors), indices 0-15.
	ANSI [16]string
	// BoldAsBright renders bold text in colors 0-7 with the bright 8-15 colors.
	BoldAsBright bool
}

// Default is the built-in dark Theme.
func Default() Theme {
	return Theme{Name: "dark", Foreground: "#c5c8c6", Background: "#1d1f21",
		ANSI: [16]string{
			"#1d1f21", "#cc6666", "#b5bd68", "#f0c674", "#81a2be", "#b294bb", "#8abeb7", "#c5c8c6",
			"#969896", "#d54e53", "#b9ca4a", "#e7c547", "#7aa6da", "#c397d8", "#70c0b1", "#eaeaea",
		},
		BoldAsBright: true,
	}
}

// Lookup returns the Theme with the given name.
func Lookup(name string) (Theme, bool) {
	if name == "dark" {
		return Default(), true
	}
	return Theme{}, false
}

// Names lists the names of all available Themes.
func Names() []string {
	return []string{"dark"}
}
