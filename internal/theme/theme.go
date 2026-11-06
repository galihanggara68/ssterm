// Package theme defines the look of a Screenshot.
package theme

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Chrome defines the Chrome (title bar and buttons) for a Theme.
type Chrome struct {
	Layout     string    `json:"style,omitempty"`
	Background string    `json:"background,omitempty"`
	Foreground string    `json:"foreground,omitempty"`
	Buttons    [3]string `json:"buttons,omitempty"`
}

// Theme is a pluggable look for a specific terminal.
type Theme struct {
	Name       string `json:"name,omitempty"`
	Foreground string `json:"foreground"` // CSS color
	Background string `json:"background"` // CSS color
	// ANSI holds the 16 ANSI colors (CSS colors), indices 0-15.
	ANSI [16]string `json:"ansi"`
	// BoldAsBright renders bold text in colors 0-7 with the bright 8-15 colors.
	BoldAsBright bool `json:"bold_as_bright"`
	// Chrome defines the Chrome drawn when --chrome is enabled.
	Chrome Chrome `json:"chrome,omitempty"`
}

var builtins = []Theme{
	{
		Name: "dark", Foreground: "#c5c8c6", Background: "#1d1f21",
		ANSI: [16]string{
			"#1d1f21", "#cc6666", "#b5bd68", "#f0c674", "#81a2be", "#b294bb", "#8abeb7", "#c5c8c6",
			"#969896", "#d54e53", "#b9ca4a", "#e7c547", "#7aa6da", "#c397d8", "#70c0b1", "#eaeaea",
		},
		BoldAsBright: true,
		Chrome: Chrome{
			Layout:     "macos",
			Background: "#2d2f31",
			Foreground: "#c5c8c6",
			Buttons:    [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
		},
	},
	{
		Name: "light", Foreground: "#4d4d4c", Background: "#ffffff",
		ANSI: [16]string{
			"#000000", "#c82829", "#718c00", "#eab700", "#4271ae", "#8959a8", "#3e999f", "#d6d6d6",
			"#969896", "#d54e53", "#b9ca4a", "#e7c547", "#7aa6da", "#c397d8", "#70c0b1", "#ffffff",
		},
		BoldAsBright: false,
		Chrome: Chrome{
			Layout:     "macos",
			Background: "#e8e8e8",
			Foreground: "#4d4d4c",
			Buttons:    [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
		},
	},
	{
		Name: "macos", Foreground: "#000000", Background: "#ffffff",
		ANSI: [16]string{
			"#000000", "#990000", "#00a600", "#999900", "#0000b2", "#b200b2", "#00a6b2", "#bfbfbf",
			"#666666", "#e50000", "#00d900", "#e5e500", "#0000ff", "#e500e5", "#00e5e5", "#e5e5e5",
		},
		BoldAsBright: true,
		Chrome: Chrome{
			Layout:     "macos",
			Background: "#e5e5e5",
			Foreground: "#333333",
			Buttons:    [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
		},
	},
	{
		Name: "iterm2", Foreground: "#c7c7c7", Background: "#000000",
		ANSI: [16]string{
			"#000000", "#c91b00", "#00c200", "#c7c400", "#0225c7", "#ca30c7", "#00c5c7", "#c7c7c7",
			"#686868", "#ff6e67", "#5ffa68", "#fffc67", "#6871ff", "#ff77ff", "#60fdff", "#ffffff",
		},
		BoldAsBright: true,
		Chrome: Chrome{
			Layout:     "macos",
			Background: "#323232",
			Foreground: "#c7c7c7",
			Buttons:    [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
		},
	},
	{
		Name: "gnome", Foreground: "#d3d7cf", Background: "#2e3436",
		ANSI: [16]string{
			"#2e3436", "#cc0000", "#4e9a06", "#c4a000", "#3465a4", "#75507b", "#06989a", "#d3d7cf",
			"#555753", "#ef2929", "#8ae234", "#fce94f", "#729fcf", "#ad7fa8", "#34e2e2", "#eeeeec",
		},
		BoldAsBright: false,
		Chrome: Chrome{
			Layout:     "gnome",
			Background: "#303030",
			Foreground: "#eeeeec",
			Buttons:    [3]string{"#4a4a4a", "#4a4a4a", "#cc0000"},
		},
	},
	{
		Name: "windows", Foreground: "#cccccc", Background: "#0c0c0c",
		ANSI: [16]string{
			"#0c0c0c", "#c50f1f", "#13a10e", "#c19c00", "#0037da", "#881798", "#3a96dd", "#cccccc",
			"#767676", "#e74856", "#16c60c", "#f9f1a5", "#3b78ff", "#b4009e", "#61d6d6", "#f2f2f2",
		},
		BoldAsBright: true,
		Chrome: Chrome{
			Layout:     "windows",
			Background: "#1f1f1f",
			Foreground: "#cccccc",
			Buttons:    [3]string{"#cccccc", "#cccccc", "#e81123"},
		},
	},
}

var aliases = map[string]string{
	"default-dark":     "dark",
	"default-light":    "light",
	"macos-terminal":   "macos",
	"gnome-terminal":   "gnome",
	"windows-terminal": "windows",
}

// Default is the built-in dark Theme.
func Default() Theme {
	return builtins[0]
}

// Lookup returns the Theme with the given name.
func Lookup(name string) (Theme, bool) {
	key := strings.Join(strings.Fields(strings.ToLower(name)), "-")
	if canon, ok := aliases[key]; ok {
		key = canon
	}
	for _, t := range builtins {
		if t.Name == key {
			return t, true
		}
	}
	return Theme{}, false
}

// Names lists the names of all available Themes.
func Names() []string {
	out := make([]string, len(builtins))
	for i, t := range builtins {
		out[i] = t.Name
	}
	return out
}

// Resolve looks up a built-in Theme by name, or loads a JSON Theme file when
// spec ends with ".json" or contains a path separator.
func Resolve(spec string) (Theme, error) {
	if isFilePath(spec) {
		return LoadFile(spec)
	}
	if t, ok := Lookup(spec); ok {
		return t, nil
	}
	return Theme{}, fmt.Errorf("unknown theme %q (valid themes: %s)", spec, strings.Join(Names(), ", "))
}

func isFilePath(spec string) bool {
	lower := strings.ToLower(spec)
	return strings.HasSuffix(lower, ".json") ||
		strings.ContainsRune(spec, '/') ||
		strings.ContainsRune(spec, '\\') ||
		strings.ContainsRune(spec, os.PathSeparator)
}

// LoadFile reads and validates a user Theme from a JSON file.
func LoadFile(path string) (Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Theme{}, err
	}
	raw, err := parseObjectKeys(path, "", data, "name", "foreground", "background", "ansi", "bold_as_bright", "chrome")
	if err != nil {
		return Theme{}, err
	}

	var t Theme
	if v, ok := raw["name"]; ok {
		if isJSONNull(v) || json.Unmarshal(v, &t.Name) != nil {
			return Theme{}, fmt.Errorf("theme %s: invalid field \"name\": expected string", path)
		}
	}
	if t.Name == "" {
		base := filepath.Base(path)
		t.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	if t.Foreground, err = requireHexField(path, raw, "foreground"); err != nil {
		return Theme{}, err
	}
	if t.Background, err = requireHexField(path, raw, "background"); err != nil {
		return Theme{}, err
	}

	ansiRaw, ok := raw["ansi"]
	if !ok {
		return Theme{}, fmt.Errorf("theme %s: missing required field \"ansi\"", path)
	}
	var ansiElems []json.RawMessage
	if isJSONNull(ansiRaw) || json.Unmarshal(ansiRaw, &ansiElems) != nil || len(ansiElems) != 16 {
		return Theme{}, fmt.Errorf("theme %s: invalid field \"ansi\": expected array of 16 #RRGGBB hex colors", path)
	}
	for i, elem := range ansiElems {
		c, err := parseHexValue(path, fmt.Sprintf("ansi[%d]", i), elem)
		if err != nil {
			return Theme{}, err
		}
		t.ANSI[i] = c
	}

	if v, ok := raw["bold_as_bright"]; ok {
		if isJSONNull(v) || json.Unmarshal(v, &t.BoldAsBright) != nil {
			return Theme{}, fmt.Errorf("theme %s: invalid field \"bold_as_bright\": expected boolean", path)
		}
	}

	t.Chrome = Default().Chrome
	if v, ok := raw["chrome"]; ok {
		chrome, err := parseChrome(path, v)
		if err != nil {
			return Theme{}, err
		}
		t.Chrome = chrome
	}

	return t, nil
}

func parseChrome(path string, data json.RawMessage) (Chrome, error) {
	raw, err := parseObjectKeys(path, "chrome", data, "style", "background", "foreground", "buttons")
	if err != nil {
		return Chrome{}, err
	}
	c := Default().Chrome
	if v, ok := raw["style"]; ok {
		if isJSONNull(v) || json.Unmarshal(v, &c.Layout) != nil || c.Layout == "" {
			return Chrome{}, fmt.Errorf("theme %s: invalid field \"chrome.style\": expected non-empty string", path)
		}
	}
	if v, ok := raw["background"]; ok {
		if c.Background, err = parseHexValue(path, "chrome.background", v); err != nil {
			return Chrome{}, err
		}
	}
	if v, ok := raw["foreground"]; ok {
		if c.Foreground, err = parseHexValue(path, "chrome.foreground", v); err != nil {
			return Chrome{}, err
		}
	}
	if v, ok := raw["buttons"]; ok {
		var elems []json.RawMessage
		if isJSONNull(v) || json.Unmarshal(v, &elems) != nil || len(elems) != 3 {
			return Chrome{}, fmt.Errorf("theme %s: invalid field \"chrome.buttons\": expected array of 3 #RRGGBB hex colors", path)
		}
		for i, elem := range elems {
			b, err := parseHexValue(path, fmt.Sprintf("chrome.buttons[%d]", i), elem)
			if err != nil {
				return Chrome{}, err
			}
			c.Buttons[i] = b
		}
	}
	return c, nil
}

func parseObjectKeys(path, prefix string, data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	if isJSONNull(data) {
		if prefix != "" {
			return nil, fmt.Errorf("theme %s: invalid field %q: expected object", path, prefix)
		}
		return nil, fmt.Errorf("theme %s: invalid JSON: expected object", path)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		if prefix != "" {
			return nil, fmt.Errorf("theme %s: invalid field %q: expected object", path, prefix)
		}
		return nil, fmt.Errorf("theme %s: invalid JSON: %w", path, err)
	}
	known := make(map[string]bool, len(allowed))
	for _, k := range allowed {
		known[k] = true
	}
	for k := range raw {
		if !known[k] {
			field := k
			if prefix != "" {
				field = prefix + "." + k
			}
			return nil, fmt.Errorf("theme %s: unknown field %q", path, field)
		}
	}
	return raw, nil
}

func requireHexField(path string, raw map[string]json.RawMessage, field string) (string, error) {
	v, ok := raw[field]
	if !ok {
		return "", fmt.Errorf("theme %s: missing required field %q", path, field)
	}
	return parseHexValue(path, field, v)
}

func parseHexValue(path, field string, raw json.RawMessage) (string, error) {
	var s string
	if isJSONNull(raw) || json.Unmarshal(raw, &s) != nil || !isValidHexColor(s) {
		return "", fmt.Errorf("theme %s: invalid field %q: expected #RRGGBB hex color", path, field)
	}
	return s, nil
}

func isJSONNull(raw []byte) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func isValidHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for i := 1; i < 7; i++ {
		ch := s[i]
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}
