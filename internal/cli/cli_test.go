package cli_test

import (
	"bytes"
	"encoding/xml"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ssterm/internal/cli"
)

var update = flag.Bool("update", false, "rewrite golden files")

func run(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	code = cli.Run(args, strings.NewReader(stdin), &out, &errb)
	return out.String(), errb.String(), code
}

// svgTexts parses the SVG and returns the content of every <text> element.
func svgTexts(t *testing.T, svg string) []string {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(svg))
	var texts []string
	inText := false
	var cur strings.Builder
	sawRoot := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if e.Name.Local == "svg" {
				sawRoot = true
			}
			if e.Name.Local == "text" {
				inText = true
				cur.Reset()
			}
		case xml.CharData:
			if inText {
				cur.Write(e)
			}
		case xml.EndElement:
			if e.Name.Local == "text" {
				inText = false
				texts = append(texts, cur.String())
			}
		}
	}
	if !sawRoot {
		t.Fatalf("output is not an SVG document:\n%s", svg)
	}
	return texts
}

// svgTextLengths returns the textLength attribute of every <text> element.
func svgTextLengths(t *testing.T, svg string) []string {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(svg))
	var out []string
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		if e, ok := tok.(xml.StartElement); ok && e.Name.Local == "text" {
			for _, a := range e.Attr {
				if a.Name.Local == "textLength" {
					out = append(out, a.Value)
				}
			}
		}
	}
}

func TestPipedPlainTextProducesSVGWithSelectableText(t *testing.T) {
	out, _, code := run(t, "hello\nworld\n")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	texts := svgTexts(t, out)
	if len(texts) != 2 || texts[0] != "hello" || texts[1] != "world" {
		t.Fatalf("texts = %q, want [hello world]", texts)
	}
}

func TestOutputFlagWritesSVGFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shot.svg")
	out, _, code := run(t, "hi\n", "-o", path)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if out != "" {
		t.Fatalf("stdout = %q, want empty when -o is given", out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if texts := svgTexts(t, string(data)); len(texts) != 1 || texts[0] != "hi" {
		t.Fatalf("texts = %q, want [hi]", texts)
	}
}

func TestSpecialCharactersAreEscaped(t *testing.T) {
	out, _, _ := run(t, "a<b & c>d\n")
	if texts := svgTexts(t, out); len(texts) != 1 || texts[0] != "a<b & c>d" {
		t.Fatalf("texts = %q, want the original line round-tripped", texts)
	}
}

func TestHelpListsFlags(t *testing.T) {
	_, stderr, code := run(t, "", "--help")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	for _, f := range []string{"-o", "--theme", "--cols", "--rows", "--font"} {
		if !strings.Contains(stderr, f) {
			t.Errorf("help does not mention %s:\n%s", f, stderr)
		}
	}
}

func golden(t *testing.T, name, stdin string, args ...string) {
	t.Helper()
	out, _, code := run(t, stdin, args...)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	golden := "testdata/" + name + ".svg"
	if *update {
		if err := os.WriteFile(golden, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if out != string(want) {
		t.Fatalf("output differs from %s (run with -update after reviewing)\n got:\n%s\nwant:\n%s", golden, out, want)
	}
}

func TestGolden(t *testing.T) {
	in, err := os.ReadFile("testdata/plain.txt")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "plain", string(in))
}

func TestColorsAndAttributesGolden(t *testing.T) {
	in, err := os.ReadFile("testdata/styled.ansi")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "styled", string(in))
}

func TestColorSpecsAreRendered(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"ansi from theme", "\x1b[31mx", `fill="#cc6666"`},
		{"256 as emitted", "\x1b[38;5;196mx", `fill="#ff0000"`},
		{"truecolor as emitted", "\x1b[38;2;18;52;86mx", `fill="#123456"`},
		{"background", "\x1b[44mx", `<rect x="0" y="0" width="9" height="18" fill="#81a2be"/>`},
		{"bold as bright", "\x1b[1;31mx", `fill="#d54e53" font-weight="bold"`},
		{"italic", "\x1b[3mx", `font-style="italic"`},
		{"underline", "\x1b[4mx", `text-decoration="underline"`},
		{"strikethrough", "\x1b[9mx", `text-decoration="line-through"`},
		{"dim", "\x1b[2mx", `opacity="0.5"`},
		{"reverse", "\x1b[7mx", `fill="#1d1f21"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, _ := run(t, tt.in)
			if !strings.Contains(out, tt.want) {
				t.Fatalf("output lacks %s:\n%s", tt.want, out)
			}
		})
	}
	out, _, _ := run(t, "\x1b[5mx")
	if strings.Contains(out, "blink") || strings.Contains(out, "<animate") {
		t.Fatalf("blink must render static:\n%s", out)
	}
}

func assertTexts(t *testing.T, stdin string, want []string, args ...string) {
	t.Helper()
	out, _, code := run(t, stdin, args...)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got := svgTexts(t, out)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("texts = %q, want %q", got, want)
	}
}

func TestCarriageReturnOverwritesYieldFinalLine(t *testing.T) {
	assertTexts(t, "10%\r50%\r100%\ndone\n", []string{"100%", "done"})
}

func TestCarriageReturnShorterOverwriteKeepsTail(t *testing.T) {
	assertTexts(t, "abcdef\rXY\n", []string{"XYcdef"})
	assertTexts(t, "ab\x1b[H\x1b[0J\x1b[3Jz", []string{"z"}, "--cols", "4", "--rows", "2")
}

func TestCursorMovementAndErase(t *testing.T) {
	assertTexts(t, "one\ntwo\nthree\x1b[2A\x1b[2K\rTWO\x1b[B\x1b[3D\x1b[K!!", []string{"TWO", "!!", "three"})
	assertTexts(t, "hello world\x1b[6D\x1b[K", []string{"hello"}, "--cols", "20")
	assertTexts(t, "ab\x1b[2J\x1b[Hz", []string{"z"}, "--cols", "4", "--rows", "2")
	assertTexts(t, "ab\x1b[H\x1b[0J\x1b[3Jz", []string{"z"}, "--cols", "4", "--rows", "2")
}

func TestScrollbackIncludedByDefault(t *testing.T) {
	// Narrow width causes 1 line to wrap into 3 rows on a 2-row auto-sized
	// grid (since line count is 2), pushing 1 row into Scrollback while
	// --rows is not set.
	assertTexts(t, "111122223333\n4444\n", []string{"1111", "2222", "3333", "4444"}, "--cols", "4")
}

func TestAlternateScreenYieldsFinalGrid(t *testing.T) {
	assertTexts(t, "main\n\x1b[?1049h\x1b[Halt\x1b[2;1Hscreen", []string{"alt", "screen"}, "--rows", "2")
	assertTexts(t, "main\n\x1b[?1049h\x1b[Halt\x1b[?1049l", []string{"main"}, "--rows", "2")
}

func TestGoldenCursorOutput(t *testing.T) {
	golden(t, "progress", "Downloading\n 10%\r 55%\r100%\nstatus: ok\x1b[3D\x1b[Kdone\n1\n2\n3\n")
}

func TestSizingAndErrors(t *testing.T) {
	tests := []struct {
		name       string
		stdin      string
		args       []string
		wantCode   int
		wantTexts  []string // checked when wantCode == 0
		wantStderr []string
	}{
		{name: "auto-size longest line", stdin: "a\nlonger line\nb\n", wantTexts: []string{"a", "longer line", "b"}},
		{name: "rows keeps last N lines", stdin: "1\n2\n3\n4\n", args: []string{"--rows", "2"}, wantTexts: []string{"3", "4"}},
		{name: "rows larger than content", stdin: "1\n2\n", args: []string{"--rows", "5"}, wantTexts: []string{"1", "2"}},
		{name: "empty stdin", stdin: "", wantCode: 1, wantStderr: []string{"ssterm:", "no input"}},
		{name: "unknown flag", stdin: "x\n", args: []string{"--bogus"}, wantCode: 2, wantStderr: []string{"bogus", "Usage:"}},
		{name: "unknown theme lists names", stdin: "x\n", args: []string{"--theme", "nope"}, wantCode: 2, wantStderr: []string{"nope", "dark"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, stderr, code := run(t, tc.stdin, tc.args...)
			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr %q)", code, tc.wantCode, stderr)
			}
			if code != 0 && out != "" {
				t.Errorf("stdout = %q, want empty on failure", out)
			}
			for _, s := range tc.wantStderr {
				if !strings.Contains(stderr, s) {
					t.Errorf("stderr missing %q:\n%s", s, stderr)
				}
			}
			if code == 0 {
				got := svgTexts(t, out)
				if strings.Join(got, "|") != strings.Join(tc.wantTexts, "|") {
					t.Errorf("texts = %q, want %q", got, tc.wantTexts)
				}
			}
		})
	}
}

func TestBadFlagPrintsUsageOnce(t *testing.T) {
	_, stderr, _ := run(t, "x\n", "--bogus")
	if n := strings.Count(stderr, "Usage:"); n != 1 {
		t.Fatalf("usage printed %d times, want 1:\n%s", n, stderr)
	}
}

func TestColsOverridesWidth(t *testing.T) {
	narrow, _, _ := run(t, "abc\n", "--cols", "3")
	wide, _, _ := run(t, "abc\n", "--cols", "30")
	if len(narrow) == 0 || narrow == wide {
		t.Fatalf("--cols did not change the output")
	}
	if texts := svgTexts(t, narrow); len(texts) != 1 || texts[0] != "abc" {
		t.Fatalf("texts = %q", texts)
	}
}

func TestWideCharactersOccupyTwoCells(t *testing.T) {
	out, _, code := run(t, "日本語x\nab\n")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	texts := svgTexts(t, out)
	if len(texts) != 2 || texts[0] != "日本語x" || texts[1] != "ab" {
		t.Fatalf("texts = %q, want [日本語x ab]", texts)
	}
	// 7 columns * 9px; the width must come from display columns, not runes.
	if got := svgTextLengths(t, out); len(got) != 2 || got[0] != "63" {
		t.Fatalf("textLengths = %q, want first 63", got)
	}
	if !strings.Contains(out, `width="63"`) {
		t.Fatalf("screen is not 7 columns wide:\n%s", out)
	}
}

func TestGoldenWideAndBoxDrawing(t *testing.T) {
	in, err := os.ReadFile("testdata/wide.txt")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "wide", string(in))
}
