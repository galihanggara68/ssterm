package cli_test

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/creack/pty"

	"ssterm/internal/cli"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "ssterm-test-xdg-*")
	if err == nil {
		_ = os.Setenv("XDG_CONFIG_HOME", tmp)
		_ = os.Unsetenv("SSTERM_CONFIG")
	}
	code := m.Run()
	if tmp != "" {
		_ = os.RemoveAll(tmp)
	}
	os.Exit(code)
}

func run(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	code = cli.Run(args, strings.NewReader(stdin), &out, &errb)
	return out.String(), errb.String(), code
}

// svgTexts parses the SVG and returns the concatenated text of every row
// (multiple <text> elements with the same y attribute are joined in order).
func svgTexts(t *testing.T, svg string) []string {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(svg))
	var texts []string
	var ys []string
	inText := false
	var curY string
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
				curY = ""
				for _, a := range e.Attr {
					if a.Name.Local == "y" {
						curY = a.Value
					}
				}
				cur.Reset()
			}
		case xml.CharData:
			if inText {
				cur.Write(e)
			}
		case xml.EndElement:
			if e.Name.Local == "text" {
				inText = false
				if n := len(ys); n > 0 && ys[n-1] == curY {
					texts[n-1] += cur.String()
				} else {
					ys = append(ys, curY)
					texts = append(texts, cur.String())
				}
			}
		}
	}
	if !sawRoot {
		t.Fatalf("output is not an SVG document:\n%s", svg)
	}
	return texts
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
	for _, f := range []string{"-o", "--theme", "--cols", "--rows", "--font", "--trim", "--crop", "--padding", "--chrome", "--title", "--shadow", "--shell", "-s", "--version", "-v"} {
		if !strings.Contains(stderr, f) {
			t.Errorf("help does not mention %s:\n%s", f, stderr)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	for _, flagName := range []string{"--version", "-v"} {
		out, stderr, code := run(t, "", flagName)
		if code != 0 {
			t.Fatalf("%s: exit code = %d, want 0 (stderr: %q)", flagName, code, stderr)
		}
		if stderr != "" {
			t.Fatalf("%s: unexpected stderr %q", flagName, stderr)
		}
		if out != "ssterm dev\n" {
			t.Fatalf("%s: stdout = %q, want %q", flagName, out, "ssterm dev\n")
		}
	}

	orig := cli.Version
	t.Cleanup(func() { cli.Version = orig })
	cli.Version = "v0.1.0"
	out, stderr, code := run(t, "", "--version")
	if code != 0 || stderr != "" || out != "ssterm v0.1.0\n" {
		t.Fatalf("custom Version: code=%d out=%q stderr=%q", code, out, stderr)
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
	wantStr := strings.ReplaceAll(string(want), "\r\n", "\n")
	if out != wantStr {
		t.Fatalf("output differs from %s (run with -update after reviewing)\n got:\n%s\nwant:\n%s", golden, out, wantStr)
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
		{"256 low index exact not theme", "\x1b[1;38;5;1mx", `fill="#800000" font-weight="bold"`},
		{"truecolor as emitted", "\x1b[38;2;18;52;86mx", `fill="#123456"`},
		{"background", "\x1b[44mx", `<rect x="0" y="0" width="9" height="18" fill="#81a2be"/>`},
		{"bold as bright", "\x1b[1;31mx", `fill="#d54e53" font-weight="bold"`},
		{"italic", "\x1b[3mx", `font-style="italic"`},
		{"underline", "\x1b[4mx", `text-decoration="underline"`},
		{"trailing underlined space kept", "a\x1b[4m \x1b[0m", `text-decoration="underline"> </tspan>`},
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
	if !slices.Equal(got, want) {
		t.Fatalf("texts = %q, want %q", got, want)
	}
}

func TestCarriageReturnOverwritesYieldFinalLine(t *testing.T) {
	assertTexts(t, "10%\r50%\r100%\ndone\n", []string{"100%", "done"})
}

func TestCarriageReturnShorterOverwriteKeepsTail(t *testing.T) {
	assertTexts(t, "abcdef\rXY\n", []string{"XYcdef"})
}

func TestCursorMovementAndErase(t *testing.T) {
	assertTexts(t, "one\ntwo\nthree\x1b[2A\x1b[2K\rTWO\x1b[B\x1b[3D\x1b[K!!", []string{"TWO", "!!", "three"})
	assertTexts(t, "hello world\x1b[6D\x1b[K", []string{"hello"}, "--cols", "20")
	assertTexts(t, "ab\x1b[2J\x1b[Hz", []string{"z"}, "--cols", "4", "--rows", "2")
	assertTexts(t, "ab\x1b[H\x1b[0J\x1b[3Jz", []string{"z"}, "--cols", "4", "--rows", "2")
}

func TestScrollbackIncludedByDefault(t *testing.T) {
	assertTexts(t, "111122223333\n4444\n", []string{"1111", "2222", "3333", "4444"}, "--cols", "4")
}

func TestAlternateScreenYieldsFinalGrid(t *testing.T) {
	assertTexts(t, "1\n2\n3\n\x1b[?1049h\x1b[Halt\x1b[2;1Hscreen", []string{"alt", "screen"}, "--rows", "2")
	assertTexts(t, "main\n\x1b[?1049h\x1b[Halt\x1b[?1049l", []string{"main"}, "--rows", "2")
	assertTexts(t, "\x1b[?1049h\x1b[Htop\x1b[2;1Hbot", []string{"top", "bot"})
}

func TestAutoSizeFitsPostEmulationContent(t *testing.T) {
	out, _, code := run(t, "status: ok\x1b[3D\x1b[Kdone\n")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, `width="99"`) {
		t.Fatalf("expected auto-sized width 99 (11 cols), got:\n%s", out)
	}
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
		{name: "whitespace stdin valid", stdin: "   \n", wantTexts: []string{}},
		{name: "empty stdin", stdin: "", wantCode: 1, wantStderr: []string{"ssterm:", "empty stdin"}},
		{name: "negative cols rejected", stdin: "x\n", args: []string{"--cols", "-5"}, wantCode: 2, wantStderr: []string{"--cols"}},
		{name: "zero rows rejected", stdin: "x\n", args: []string{"--rows", "0"}, wantCode: 2, wantStderr: []string{"--rows"}},
		{name: "invalid crop syntax rejected", stdin: "x\n", args: []string{"--crop", "bad"}, wantCode: 2, wantStderr: []string{"--crop"}},
		{name: "negative crop origin rejected", stdin: "x\n", args: []string{"--crop", "-1,0,2x2"}, wantCode: 2, wantStderr: []string{"--crop"}},
		{name: "zero crop size rejected", stdin: "x\n", args: []string{"--crop", "0,0,0x2"}, wantCode: 2, wantStderr: []string{"--crop"}},
		{name: "negative padding rejected", stdin: "x\n", args: []string{"--padding", "-1"}, wantCode: 2, wantStderr: []string{"--padding"}},
		{name: "unknown flag", stdin: "x\n", args: []string{"--bogus"}, wantCode: 2, wantStderr: []string{"bogus", "Usage:"}},
		{name: "unknown theme lists names", stdin: "x\n", args: []string{"--theme", "nope"}, wantCode: 2, wantStderr: []string{"nope", "dark"}},
		{name: "unsupported output extension", stdin: "x\n", args: []string{"-o", "out.jpg"}, wantCode: 2, wantStderr: []string{"ssterm:", ".jpg"}},
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
				if !slices.Equal(got, tc.wantTexts) {
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
	if !strings.Contains(out, `width="63"`) {
		t.Fatalf("screen is not 7 columns wide:\n%s", out)
	}
	if !strings.Contains(out, `<text x="0" y="13" textLength="54"`) || !strings.Contains(out, `<text x="54" y="13" textLength="9"`) {
		t.Fatalf("expected wide and narrow segments anchored at x=0 and x=54:\n%s", out)
	}
}

func TestGoldenWideAndBoxDrawing(t *testing.T) {
	in, err := os.ReadFile("testdata/wide.txt")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "wide", string(in))
}

func TestOSC8HyperlinksBecomeSVGLinks(t *testing.T) {
	in := "visit \x1b]8;id=1;https://example.com/a?x=1&y=2\x1b\\example\x1b]8;;\x1b\\ or \x1b]8;;https://go.dev\x07\x1b[4;34mgo.dev\x1b[0m\x1b]8;;\x07 now\n"
	out, _, code := run(t, in)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got := svgTexts(t, out); !slices.Equal(got, []string{"visit example or go.dev now"}) {
		t.Fatalf("texts = %q, want [visit example or go.dev now]", got)
	}
	if !strings.Contains(out, `width="243"`) {
		t.Fatalf("expected auto-sized width 243 (27 cols), got:\n%s", out)
	}
	for _, want := range []string{
		`<a href="https://example.com/a?x=1&amp;y=2">example</a>`,
		`<a href="https://go.dev"><tspan fill="#81a2be" text-decoration="underline">go.dev</tspan></a>`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %s:\n%s", want, out)
		}
	}
}

func TestFontFlagSetsFontFamily(t *testing.T) {
	out, _, code := run(t, "hello\n", "--font", "JetBrains Mono, 'Fira Code', monospace")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	want := `font-family="JetBrains Mono, &#39;Fira Code&#39;, monospace"`
	if !strings.Contains(out, want) {
		t.Fatalf("output missing %s:\n%s", want, out)
	}
}

func TestHyperlinksAndFontGolden(t *testing.T) {
	in, err := os.ReadFile("testdata/links.ansi")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "links", string(in), "--font", "JetBrains Mono, monospace")
}

func runPNG(t *testing.T, stdin string, args ...string) (image.Image, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shot.png")
	allArgs := append([]string{"-o", path}, args...)
	out, stderr, code := run(t, stdin, allArgs...)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if out != "" {
		t.Fatalf("stdout = %q, want empty when -o is given", out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png.Decode: %v", err)
	}
	return img, data
}

func TestOutputFlagWritesPNGWithMatchingDimensions(t *testing.T) {
	img, _ := runPNG(t, "hello\nworld\n")
	if got := img.Bounds().Size(); got != image.Pt(5*9, 2*18) {
		t.Fatalf("size = %v, want %v", got, image.Pt(5*9, 2*18))
	}
	got := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
	want := color.NRGBA{R: 0x1d, G: 0x1f, B: 0x21, A: 0xff}
	if got != want {
		t.Fatalf("bg pixel at (0,0) = %#v, want %#v", got, want)
	}
}

func cellColors(img image.Image, col, row int) map[color.NRGBA]int {
	counts := make(map[color.NRGBA]int)
	for y := row * 18; y < (row+1)*18; y++ {
		for x := col * 9; x < (col+1)*9; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			counts[c]++
		}
	}
	return counts
}

func TestPNGColorsAndAttributes(t *testing.T) {
	darkBg := color.NRGBA{R: 0x1d, G: 0x1f, B: 0x21, A: 0xff}
	darkFg := color.NRGBA{R: 0xc5, G: 0xc8, B: 0xc6, A: 0xff}
	ansiRed := color.NRGBA{R: 0xcc, G: 0x66, B: 0x66, A: 0xff}
	ansiBlue := color.NRGBA{R: 0x81, G: 0xa2, B: 0xbe, A: 0xff}
	brightRed := color.NRGBA{R: 0xd5, G: 0x4e, B: 0x53, A: 0xff}

	t.Run("ansi fg and bg", func(t *testing.T) {
		img, _ := runPNG(t, "\x1b[31;44mM\x1b[0m \n", "--cols", "2")
		c0 := cellColors(img, 0, 0)
		if c0[ansiBlue] == 0 {
			t.Fatalf("cell (0,0) missing blue background %#v: %v", ansiBlue, c0)
		}
		if c0[ansiRed] == 0 {
			t.Fatalf("cell (0,0) missing red foreground %#v: %v", ansiRed, c0)
		}
		c1 := cellColors(img, 1, 0)
		if c1[darkBg] != 9*18 {
			t.Fatalf("blank cell (1,0) should be all default bg %#v: %v", darkBg, c1)
		}
	})

	t.Run("256-color and truecolor", func(t *testing.T) {
		img, _ := runPNG(t, "\x1b[38;5;196mM\x1b[38;2;18;52;86mM\n")
		if cellColors(img, 0, 0)[color.NRGBA{R: 0xff, G: 0x00, B: 0x00, A: 0xff}] == 0 {
			t.Fatalf("cell (0,0) missing 256-color #ff0000")
		}
		if cellColors(img, 1, 0)[color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}] == 0 {
			t.Fatalf("cell (1,0) missing truecolor #123456")
		}
	})

	t.Run("bold as bright and font variants", func(t *testing.T) {
		img, _ := runPNG(t, "\x1b[1;31mM\x1b[0;3mM\n")
		if cellColors(img, 0, 0)[brightRed] == 0 {
			t.Fatalf("bold palette 1 should use bright red %#v: %v", brightRed, cellColors(img, 0, 0))
		}
		if cellColors(img, 1, 0)[darkFg] == 0 {
			t.Fatalf("italic cell missing foreground %#v", darkFg)
		}
	})

	t.Run("reverse and dim", func(t *testing.T) {
		img, _ := runPNG(t, "\x1b[7mM\x1b[0;2mM\n")
		rev := cellColors(img, 0, 0)
		if rev[darkFg] == 0 || rev[darkBg] == 0 {
			t.Fatalf("reverse cell should have swapped fg/bg: %v", rev)
		}
		if got := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA); got != darkFg {
			t.Fatalf("reversed cell corner = %#v, want %#v", got, darkFg)
		}
		dimFg := color.NRGBA{
			R: uint8((uint16(darkFg.R) + uint16(darkBg.R)) / 2),
			G: uint8((uint16(darkFg.G) + uint16(darkBg.G)) / 2),
			B: uint8((uint16(darkFg.B) + uint16(darkBg.B)) / 2),
			A: 0xff,
		}
		if cellColors(img, 1, 0)[dimFg] == 0 {
			t.Fatalf("dim cell missing blended foreground %#v: %v", dimFg, cellColors(img, 1, 0))
		}
	})

	t.Run("underline and strikethrough on space", func(t *testing.T) {
		img, _ := runPNG(t, "\x1b[4m \x1b[0;9m \x1b[0;5mM\x1b[0m\n")
		if cellColors(img, 0, 0)[darkFg] < 9 {
			t.Fatalf("underlined space should draw line in fg %#v: %v", darkFg, cellColors(img, 0, 0))
		}
		if cellColors(img, 1, 0)[darkFg] < 9 {
			t.Fatalf("strikethrough space should draw line in fg %#v: %v", darkFg, cellColors(img, 1, 0))
		}
		if cellColors(img, 2, 0)[darkFg] == 0 {
			t.Fatalf("static blink cell should render foreground %#v: %v", darkFg, cellColors(img, 2, 0))
		}
	})
}

func TestPNGOutputIsDeterministicAndCaseInsensitive(t *testing.T) {
	_, b1 := runPNG(t, "\x1b[1;32mhello\x1b[0m world\nsecond line\n")
	upperPath := filepath.Join(t.TempDir(), "SHOT.PNG")
	_, stderr, code := run(t, "\x1b[1;32mhello\x1b[0m world\nsecond line\n", "-o", upperPath)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	b2, err := os.ReadFile(upperPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("two PNG runs produced different bytes (%d vs %d)", len(b1), len(b2))
	}
}

func TestBuiltinThemes(t *testing.T) {
	tests := []struct {
		theme      string
		stdin      string
		wantSubstr []string
	}{
		{
			theme:      "dark",
			stdin:      "\x1b[1;31mx\n",
			wantSubstr: []string{`fill="#1d1f21"`, `fill="#c5c8c6"`, `fill="#d54e53"`},
		},
		{
			theme:      "light",
			stdin:      "\x1b[1;31mx\n",
			wantSubstr: []string{`fill="#ffffff"`, `fill="#4d4d4c"`, `fill="#c82829"`},
		},
		{
			theme:      "macos",
			stdin:      "\x1b[31mx\n",
			wantSubstr: []string{`fill="#ffffff"`, `fill="#000000"`, `fill="#990000"`},
		},
		{
			theme:      "iterm2",
			stdin:      "\x1b[31mx\n",
			wantSubstr: []string{`fill="#000000"`, `fill="#c7c7c7"`, `fill="#c91b00"`},
		},
		{
			theme:      "gnome",
			stdin:      "\x1b[1;31mx\n",
			wantSubstr: []string{`fill="#2e3436"`, `fill="#d3d7cf"`, `fill="#cc0000"`},
		},
		{
			theme:      "windows",
			stdin:      "\x1b[31mx\n",
			wantSubstr: []string{`fill="#0c0c0c"`, `fill="#cccccc"`, `fill="#c50f1f"`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.theme, func(t *testing.T) {
			out, stderr, code := run(t, tc.stdin, "--theme", tc.theme)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
			}
			for _, want := range tc.wantSubstr {
				if !strings.Contains(out, want) {
					t.Errorf("theme %q output missing %s:\n%s", tc.theme, want, out)
				}
			}
		})
	}

	// Space-separated display names from the spec ("default dark", "macOS Terminal", etc.) resolve too.
	for _, displayName := range []string{"default dark", "default light", "macOS Terminal", "iTerm2", "GNOME", "Windows Terminal"} {
		if _, stderr, code := run(t, "x\n", "--theme", displayName); code != 0 {
			t.Errorf("display name %q failed: code=%d stderr=%s", displayName, code, stderr)
		}
	}

	// Built-in theme applies to PNG output as well.
	lightImg, _ := runPNG(t, "\x1b[31mM\n", "--theme", "light")
	if got := color.NRGBAModel.Convert(lightImg.At(0, 0)).(color.NRGBA); got != (color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}) {
		t.Fatalf("light theme PNG bg pixel = %#v, want #ffffff", got)
	}
	if cellColors(lightImg, 0, 0)[color.NRGBA{R: 0xc8, G: 0x28, B: 0x29, A: 0xff}] == 0 {
		t.Fatalf("light theme PNG missing red #c82829: %v", cellColors(lightImg, 0, 0))
	}

	_, stderr, code := run(t, "x\n", "--theme", "nonexistent")
	if code == 0 {
		t.Fatalf("expected non-zero exit code for unknown theme")
	}
	for _, name := range []string{"dark", "light", "macos", "iterm2", "gnome", "windows"} {
		if !strings.Contains(stderr, name) {
			t.Errorf("unknown theme error missing built-in %q:\n%s", name, stderr)
		}
	}
}

func TestCommunityThemes(t *testing.T) {
	communityThemes := []struct {
		name       string
		bgHex      string
		fgHex      string
		ansiRedHex string // ANSI 1
		chromeBg   string
		bgNRGBA    color.NRGBA
	}{
		{
			name:       "dracula",
			bgHex:      "#282a36",
			fgHex:      "#f8f8f2",
			ansiRedHex: "#ff5555",
			chromeBg:   "#21222c",
			bgNRGBA:    color.NRGBA{R: 0x28, G: 0x2a, B: 0x36, A: 0xff},
		},
		{
			name:       "catppuccin-mocha",
			bgHex:      "#1e1e2e",
			fgHex:      "#cdd6f4",
			ansiRedHex: "#f38ba8",
			chromeBg:   "#181825",
			bgNRGBA:    color.NRGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0xff},
		},
		{
			name:       "catppuccin-latte",
			bgHex:      "#eff1f5",
			fgHex:      "#4c4f69",
			ansiRedHex: "#d20f39",
			chromeBg:   "#e6e9ef",
			bgNRGBA:    color.NRGBA{R: 0xef, G: 0xf1, B: 0xf5, A: 0xff},
		},
		{
			name:       "tokyo-night",
			bgHex:      "#1a1b26",
			fgHex:      "#c0caf5",
			ansiRedHex: "#f7768e",
			chromeBg:   "#16161e",
			bgNRGBA:    color.NRGBA{R: 0x1a, G: 0x1b, B: 0x26, A: 0xff},
		},
		{
			name:       "nord",
			bgHex:      "#2e3440",
			fgHex:      "#d8dee9",
			ansiRedHex: "#bf616a",
			chromeBg:   "#242933",
			bgNRGBA:    color.NRGBA{R: 0x2e, G: 0x34, B: 0x40, A: 0xff},
		},
		{
			name:       "gruvbox-dark",
			bgHex:      "#282828",
			fgHex:      "#ebdbb2",
			ansiRedHex: "#cc241d",
			chromeBg:   "#1d2021",
			bgNRGBA:    color.NRGBA{R: 0x28, G: 0x28, B: 0x28, A: 0xff},
		},
	}

	for _, tt := range communityThemes {
		t.Run(tt.name+"/svg", func(t *testing.T) {
			out, stderr, code := run(t, "\x1b[1;31mhello\x1b[0m\n", "--theme", tt.name, "--chrome")
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
			}
			if !strings.Contains(out, fmt.Sprintf(`fill="%s"`, tt.bgHex)) {
				t.Errorf("SVG missing background %s:\n%s", tt.bgHex, out)
			}
			if !strings.Contains(out, fmt.Sprintf(`fill="%s"`, tt.ansiRedHex)) {
				t.Errorf("SVG missing bold ANSI 1 color %s (bold_as_bright should be false):\n%s", tt.ansiRedHex, out)
			}
			if !strings.Contains(out, fmt.Sprintf(`fill="%s"`, tt.chromeBg)) {
				t.Errorf("SVG missing chrome background %s:\n%s", tt.chromeBg, out)
			}
		})

		t.Run(tt.name+"/png", func(t *testing.T) {
			img, _ := runPNG(t, "hello\n", "--theme", tt.name)
			gotBg := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
			if gotBg != tt.bgNRGBA {
				t.Fatalf("PNG (0,0) bg = %#v, want %#v", gotBg, tt.bgNRGBA)
			}
		})
	}
}

func TestThemeDiscoveryHelpAndUnknown(t *testing.T) {
	allThemes := []string{
		"dark", "light", "macos", "iterm2", "gnome", "windows",
		"dracula", "catppuccin-mocha", "catppuccin-latte", "tokyo-night", "nord", "gruvbox-dark",
	}

	t.Run("help lists all themes", func(t *testing.T) {
		_, stderr, code := run(t, "", "--help")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		for _, name := range allThemes {
			if !strings.Contains(stderr, name) {
				t.Errorf("help missing theme %q:\n%s", name, stderr)
			}
		}
	})

	t.Run("unknown theme error and exit code 2", func(t *testing.T) {
		_, stderr, code := run(t, "x\n", "--theme", "invalid-theme")
		if code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		wantPrefix := `ssterm: unknown theme "invalid-theme", valid themes: `
		if !strings.Contains(stderr, wantPrefix) {
			t.Errorf("stderr missing %q:\n%s", wantPrefix, stderr)
		}
		for _, name := range allThemes {
			if !strings.Contains(stderr, name) {
				t.Errorf("unknown theme error missing %q:\n%s", name, stderr)
			}
		}
	})
}

func TestThemeJSONFileAndValidation(t *testing.T) {
	validJSON := `{
  "name": "house",
  "foreground": "#eeeeee",
  "background": "#112233",
  "bold_as_bright": true,
  "ansi": [
    "#000000", "#110000", "#001100", "#111100",
    "#000011", "#110011", "#001111", "#aaaaaa",
    "#555555", "#ff1122", "#11ff22", "#ffff22",
    "#1122ff", "#ff22ff", "#22ffff", "#ffffff"
  ],
  "chrome": {
    "style": "macos",
    "background": "#223344",
    "foreground": "#eeeeee",
    "buttons": ["#ff5f56", "#ffbd2e", "#27c93f"]
  }
}`
	dir := t.TempDir()
	validPath := filepath.Join(dir, "house.json")
	if err := os.WriteFile(validPath, []byte(validJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	out, stderr, code := run(t, "\x1b[31mhi\x1b[1;31m!\n", "--theme", validPath)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	for _, want := range []string{`fill="#112233"`, `fill="#eeeeee"`, `fill="#110000"`, `fill="#ff1122"`} {
		if !strings.Contains(out, want) {
			t.Errorf("custom theme SVG missing %s:\n%s", want, out)
		}
	}

	invalidCases := []struct {
		name      string
		json      string
		wantField string
	}{
		{
			name: "missing foreground",
			json: `{
				"background": "#112233",
				"ansi": ["#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000"]
			}`,
			wantField: "foreground",
		},
		{
			name: "invalid background hex",
			json: `{
				"foreground": "#ffffff",
				"background": "not-a-color",
				"ansi": ["#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000"]
			}`,
			wantField: "background",
		},
		{
			name: "ansi wrong length",
			json: `{
				"foreground": "#ffffff",
				"background": "#000000",
				"ansi": ["#000000", "#111111"]
			}`,
			wantField: "ansi",
		},
		{
			name: "ansi invalid element",
			json: `{
				"foreground": "#ffffff",
				"background": "#000000",
				"ansi": ["#000000","#000000","#000000","#GGGGGG","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000"]
			}`,
			wantField: "ansi[3]",
		},
		{
			name: "unknown top-level field",
			json: `{
				"foreground": "#ffffff",
				"background": "#000000",
				"ansi": ["#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000"],
				"bogus_field": true
			}`,
			wantField: "bogus_field",
		},
		{
			name: "wrong type on bold_as_bright",
			json: `{
				"foreground": "#ffffff",
				"background": "#000000",
				"bold_as_bright": "yes",
				"ansi": ["#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000"]
			}`,
			wantField: "bold_as_bright",
		},
		{
			name: "invalid chrome button color",
			json: `{
				"foreground": "#ffffff",
				"background": "#000000",
				"ansi": ["#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000"],
				"chrome": {"buttons": ["#ff5f56", "bad", "#27c93f"]}
			}`,
			wantField: "chrome.buttons[1]",
		},
		{
			name: "null bold_as_bright rejected",
			json: `{
				"foreground": "#ffffff",
				"background": "#000000",
				"bold_as_bright": null,
				"ansi": ["#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000"]
			}`,
			wantField: "bold_as_bright",
		},
		{
			name: "null chrome rejected",
			json: `{
				"foreground": "#ffffff",
				"background": "#000000",
				"chrome": null,
				"ansi": ["#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000"]
			}`,
			wantField: "chrome",
		},
	}

	for i, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, "bad-"+string(rune('a'+i))+".json")
			if err := os.WriteFile(p, []byte(tc.json), 0o644); err != nil {
				t.Fatal(err)
			}
			out, stderr, code := run(t, "x\n", "--theme", p)
			if code == 0 {
				t.Fatalf("expected non-zero exit code for %s, got 0", tc.name)
			}
			if out != "" {
				t.Errorf("expected empty stdout on error, got %q", out)
			}
			if !strings.Contains(stderr, tc.wantField) {
				t.Errorf("stderr %q does not name offending field %q", stderr, tc.wantField)
			}
		})
	}

	_, stderr, code = run(t, "x\n", "--theme", filepath.Join(dir, "missing.json"))
	if code == 0 || !strings.Contains(stderr, "missing.json") {
		t.Fatalf("expected error mentioning missing.json, got code=%d stderr=%q", code, stderr)
	}
}

func TestPTYRunsCommandWithTTYAndRendersFinalScreen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY mode is not supported on Windows")
	}
	out, stderr, code := run(t, "", "--", "sh", "-c", `test -t 1 && printf "\033[32mhello\033[0m\nworld\rfinal\n"`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if got := svgTexts(t, out); !slices.Equal(got, []string{"hello", "final"}) {
		t.Fatalf("texts = %q, want [hello final]", got)
	}
	if !strings.Contains(out, `fill="#b5bd68">hello</tspan>`) {
		t.Fatalf("expected green ANSI color on hello:\n%s", out)
	}
	// PTY mode auto-fits to the non-blank usedBounds (5 cols x 2 rows = 45x36 px) when --cols/--rows are not set.
	if !strings.Contains(out, `width="45" height="36"`) {
		t.Fatalf("expected auto-fit PTY size 5x2 (45x36), got:\n%s", out)
	}

	// PTY mode includes Scrollback by default when --rows is not explicitly set.
	out, stderr, code = run(t, "", "--cols", "10", "--", "sh", "-c", `for i in $(seq 1 30); do printf "line-%02d\n" "$i"; done`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	texts := svgTexts(t, out)
	if len(texts) != 30 || texts[0] != "line-01" || texts[29] != "line-30" {
		t.Fatalf("expected all 30 lines including Scrollback, got len=%d first=%q last=%q", len(texts), texts[0], texts[len(texts)-1])
	}
	if !strings.Contains(out, `width="90" height="540"`) {
		t.Fatalf("expected 10x30 size (90x540), got:\n%s", out)
	}

	// When --rows is explicitly set, Scrollback is excluded and only the final Screen grid rows are rendered.
	out, stderr, code = run(t, "", "--cols", "10", "--rows", "2", "--", "sh", "-c", `printf "1\n2\n3"`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if got := svgTexts(t, out); !slices.Equal(got, []string{"2", "3"}) {
		t.Fatalf("texts = %q, want [2 3]", got)
	}

	// Alternate screen in PTY mode excludes main-screen Scrollback and auto-fits the alternate screen content.
	out, stderr, code = run(t, "", "--", "sh", "-c", `for i in $(seq 1 30); do printf "main-%02d\n" "$i"; done; printf "\033[?1049h\033[Halt-only"`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if got := svgTexts(t, out); !slices.Equal(got, []string{"alt-only"}) {
		t.Fatalf("alt-screen PTY texts = %q, want [alt-only]", got)
	}
	if !strings.Contains(out, `width="72" height="18"`) {
		t.Fatalf("expected 8x1 auto-fit alt-screen size (72x18), got:\n%s", out)
	}

	// All-blank PTY output auto-fits to a 1x1 blank Screen grid (9x18 px).
	out, stderr, code = run(t, "", "--", "sh", "-c", `printf "   \n  \n"`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if !strings.Contains(out, `width="9" height="18"`) {
		t.Fatalf("expected 1x1 blank PTY size (9x18), got:\n%s", out)
	}
}

func TestPTYSizingFromTerminalAndFlagOverrides(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY mode is not supported on Windows")
	}
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = ptmx.Close()
		_ = tty.Close()
	}()
	if err := pty.Setsize(ptmx, &pty.Winsize{Cols: 5, Rows: 12}); err != nil {
		t.Fatal(err)
	}
	var outBuf, errBuf bytes.Buffer
	// Emulating at the controlling terminal's 5-column width wraps "12345678" into 2 rows ("12345" and "678"),
	// and auto-fits the rendered Screen grid to 5x2 (45x36 px).
	if code := cli.Run([]string{"--", "sh", "-c", `printf "12345678"`}, tty, &outBuf, &errBuf); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errBuf.String())
	}
	if got := svgTexts(t, outBuf.String()); !slices.Equal(got, []string{"12345", "678"}) {
		t.Fatalf("wrapped texts = %q, want [12345 678]", got)
	}
	if !strings.Contains(outBuf.String(), `width="45" height="36"`) {
		t.Fatalf("expected 5x2 wrapped auto-fit size (45x36), got:\n%s", outBuf.String())
	}

	out, stderr, code := run(t, "", "--cols", "4", "--rows", "2", "--", "sh", "-c", `printf "111122223333"`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if got := svgTexts(t, out); !slices.Equal(got, []string{"2222", "3333"}) {
		t.Fatalf("texts = %q, want [2222 3333]", got)
	}
	if !strings.Contains(out, `width="36" height="36"`) {
		t.Fatalf("expected 4x2 override size (36x36), got:\n%s", out)
	}
}

func TestPTYExitCodePropagationAndErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY mode is not supported on Windows")
	}
	out, stderr, code := run(t, "", "--", "sh", "-c", `printf "fail\n"; exit 7`)
	if code != 7 {
		t.Fatalf("exit code = %d, want 7 (stderr: %q)", code, stderr)
	}
	if got := svgTexts(t, out); !slices.Equal(got, []string{"fail"}) {
		t.Fatalf("texts = %q, want [fail]", got)
	}

	out, stderr, code = run(t, "", "--")
	if code == 0 || out != "" || !strings.Contains(stderr, "ssterm:") {
		t.Fatalf("missing command: code=%d out=%q stderr=%q", code, out, stderr)
	}

	out, stderr, code = run(t, "", "--", "/nonexistent/ssterm-no-such-binary")
	if code == 0 || out != "" || !strings.Contains(stderr, "ssterm:") {
		t.Fatalf("bad command: code=%d out=%q stderr=%q", code, out, stderr)
	}
}

func TestPTYShellModeLoadsProfileAliasesAndFunctions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY mode is not supported on Windows")
	}
	t.Setenv("TERM", "dumb")
	t.Setenv("COLORTERM", "")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("FORCE_COLOR", "")
	envOut, stderr, code := run(t, "", "--", "sh", "-c", `printf "%s|%s|%s|%s\n" "$TERM" "$COLORTERM" "$CLICOLOR_FORCE" "$FORCE_COLOR"`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if got := svgTexts(t, envOut); !slices.Equal(got, []string{"xterm-256color|truecolor|1|1"}) {
		t.Fatalf("color env vars = %q, want [xterm-256color|truecolor|1|1]", got)
	}

	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found in PATH")
	}
	home := t.TempDir()
	bashrc := "alias my_colored_alias='printf \"\\033[32malias-ok\\033[0m\\n\"'\n" +
		"my_bashrc_fn() { printf \"fn:%s\\n\" \"$1\"; }\n"
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(bashrc), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SHELL", bashPath)

	out, stderr, code := run(t, "", "--shell", "--", "my_colored_alias")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if got := svgTexts(t, out); !slices.Equal(got, []string{"alias-ok"}) {
		t.Fatalf("texts = %q, want [alias-ok]", got)
	}
	if !strings.Contains(out, `fill="#b5bd68">alias-ok</tspan>`) {
		t.Fatalf("expected green ANSI color from alias:\n%s", out)
	}

	out, stderr, code = run(t, "", "-s", "--", "my_bashrc_fn", "it's ok")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if got := svgTexts(t, out); !slices.Equal(got, []string{"fn:it's ok"}) {
		t.Fatalf("texts = %q, want [fn:it's ok]", got)
	}

	if zshPath, err := exec.LookPath("zsh"); err == nil {
		if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("my_zsh_fn() { printf \"zsh:%s\\n\" \"$1\"; }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SHELL", zshPath)
		zOut, stderr, code := run(t, "", "-s", "--", "my_zsh_fn", "works")
		if code != 0 {
			t.Fatalf("zsh exit code = %d, want 0 (stderr: %q)", code, stderr)
		}
		if got := svgTexts(t, zOut); !slices.Equal(got, []string{"zsh:works"}) {
			t.Fatalf("zsh texts = %q, want [zsh:works]", got)
		}
	}

	if shPath, err := exec.LookPath("sh"); err == nil {
		t.Setenv("SHELL", shPath)
		sOut, stderr, code := run(t, "", "-s", "--", "printf 'sh-fallback\n'")
		if code != 0 {
			t.Fatalf("sh exit code = %d, want 0 (stderr: %q)", code, stderr)
		}
		if !slices.Contains(svgTexts(t, sOut), "sh-fallback") {
			t.Fatalf("sh texts = %q, want to contain sh-fallback", svgTexts(t, sOut))
		}
	}
}

func TestWindowsPTYUnsupportedAndPipedMode(t *testing.T) {
	// CRLF piped Source (Windows line endings) renders cleanly on all platforms.
	out, stderr, code := run(t, "\x1b[32mwin-line-1\x1b[0m\r\nwin-line-2\r\n")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if got := svgTexts(t, out); !slices.Equal(got, []string{"win-line-1", "win-line-2"}) {
		t.Fatalf("texts = %q, want [win-line-1 win-line-2]", got)
	}
	if runtime.GOOS == "windows" {
		out, stderr, code := run(t, "", "--", "cmd", "/c", "echo hi")
		if code == 0 || out != "" {
			t.Fatalf("expected non-zero exit and empty stdout on Windows PTY, got code=%d out=%q", code, out)
		}
		want := "PTY mode (--) is not supported on Windows; pipe output to ssterm instead"
		if !strings.Contains(stderr, want) {
			t.Fatalf("Windows PTY stderr = %q, want to contain %q", stderr, want)
		}
	}
}

func TestTrimEnhancement(t *testing.T) {
	// Piped Source with leading/trailing blank rows and leading/trailing blank columns.
	in := "\n\n  hi   \n   ok  \n\n"
	out, stderr, code := run(t, in, "--cols", "10", "--rows", "8", "--trim")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if got := svgTexts(t, out); !slices.Equal(got, []string{"hi", " ok"}) {
		t.Fatalf("texts = %q, want [\"hi\" \" ok\"]", got)
	}
	if !strings.Contains(out, `width="27" height="36"`) {
		t.Fatalf("expected trimmed 3x2 size (27x36), got:\n%s", out)
	}

	// All-blank Source with --trim leaves a 1x1 Screen grid.
	blankOut, stderr, code := run(t, "   \n   \n", "--cols", "5", "--rows", "4", "--trim")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if !strings.Contains(blankOut, `width="9" height="18"`) {
		t.Fatalf("expected 1x1 blank grid (9x18), got:\n%s", blankOut)
	}

	// PNG output with --trim strips leading/trailing blank rows and columns.
	img, _ := runPNG(t, "\n  \x1b[44mM\x1b[0m  \n\n", "--cols", "6", "--rows", "4", "--trim")
	if got := img.Bounds().Size(); got != image.Pt(9, 18) {
		t.Fatalf("trimmed PNG size = %v, want (9,18)", got)
	}
	ansiBlue := color.NRGBA{R: 0x81, G: 0xa2, B: 0xbe, A: 0xff}
	if cellColors(img, 0, 0)[ansiBlue] == 0 {
		t.Fatalf("trimmed PNG cell (0,0) missing blue background: %v", cellColors(img, 0, 0))
	}
}

func TestCropEnhancement(t *testing.T) {
	grid := "012345\nabcdef\nABCDEF\nUVWXYZ\n"
	for _, spec := range []string{"1,1,3x2", "3x2+1+1", "1,1,3,2"} {
		out, stderr, code := run(t, grid, "--crop", spec)
		if code != 0 {
			t.Fatalf("--crop %s: exit code = %d, want 0 (stderr: %q)", spec, code, stderr)
		}
		if got := svgTexts(t, out); !slices.Equal(got, []string{"bcd", "BCD"}) {
			t.Fatalf("--crop %s: texts = %q, want [bcd BCD]", spec, got)
		}
		if !strings.Contains(out, `width="27" height="36"`) {
			t.Fatalf("--crop %s: expected 3x2 size (27x36), got:\n%s", spec, out)
		}
	}

	// Wide characters straddling left and right crop boundaries become spaces cleanly.
	// "a日本b": col 0='a', col 1..2='日', col 3..4='本', col 5='b'.
	// Cropping 2,0,3x1 covers cols 2..4: right half of '日' (space) + '本' (cols 1..2 of crop).
	wideOut, stderr, code := run(t, "a日本b\n", "--crop", "2,0,3x1")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if got := svgTexts(t, wideOut); !slices.Equal(got, []string{" 本"}) {
		t.Fatalf("wide crop texts = %q, want [\" 本\"]", got)
	}
	if !strings.Contains(wideOut, `width="27" height="18"`) {
		t.Fatalf("expected 3x1 size (27x18), got:\n%s", wideOut)
	}

	// PNG output with --crop extracts the target subgrid.
	img, _ := runPNG(t, "..\n.\x1b[41mM\x1b[0m\n", "--crop", "1,1,2x2")
	if got := img.Bounds().Size(); got != image.Pt(2*9, 2*18) {
		t.Fatalf("cropped PNG size = %v, want (18,36)", got)
	}
	ansiRed := color.NRGBA{R: 0xcc, G: 0x66, B: 0x66, A: 0xff}
	if cellColors(img, 0, 0)[ansiRed] == 0 {
		t.Fatalf("cropped PNG cell (0,0) missing red background: %v", cellColors(img, 0, 0))
	}
}

func TestPaddingEnhancement(t *testing.T) {
	// SVG with --padding 12 shifts background rects and text by (12, 12) and adds 24px to width/height.
	out, stderr, code := run(t, "\x1b[44mhi\x1b[0m\n", "--padding", "12")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	for _, want := range []string{
		`width="42" height="42" viewBox="0 0 42 42"`,
		`<rect width="42" height="42" fill="#1d1f21"/>`,
		`<rect x="12" y="12" width="18" height="18" fill="#81a2be"/>`,
		`<text x="12" y="25" textLength="18" lengthAdjust="spacing">hi</text>`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("padded SVG missing %s:\n%s", want, out)
		}
	}

	// PNG with --padding 10 adds 10px of Theme background around the 1x1 cell grid (9x18 -> 29x38).
	img, _ := runPNG(t, "\x1b[44mM\x1b[0m\n", "--padding", "10")
	if got := img.Bounds().Size(); got != image.Pt(9+20, 18+20) {
		t.Fatalf("padded PNG size = %v, want (29,38)", got)
	}
	darkBg := color.NRGBA{R: 0x1d, G: 0x1f, B: 0x21, A: 0xff}
	ansiBlue := color.NRGBA{R: 0x81, G: 0xa2, B: 0xbe, A: 0xff}
	for _, pt := range []image.Point{{0, 0}, {9, 9}, {10, 9}, {9, 10}, {28, 37}, {19, 28}} {
		if got := color.NRGBAModel.Convert(img.At(pt.X, pt.Y)).(color.NRGBA); got != darkBg {
			t.Fatalf("padding pixel at %v = %#v, want Theme bg %#v", pt, got, darkBg)
		}
	}
	cellContentColors := make(map[color.NRGBA]int)
	for y := 10; y < 10+18; y++ {
		for x := 10; x < 10+9; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			cellContentColors[c]++
		}
	}
	if cellContentColors[ansiBlue] == 0 {
		t.Fatalf("padded PNG cell interior missing blue background: %v", cellContentColors)
	}

	// Faithful mode: --padding 0 produces byte-for-byte identical output to no flags.
	faithfulSVG, _, _ := run(t, "hello\nworld\n")
	zeroPadSVG, _, _ := run(t, "hello\nworld\n", "--padding", "0")
	if faithfulSVG != zeroPadSVG {
		t.Fatalf("--padding 0 SVG differs from Faithful SVG")
	}
	_, faithfulPNG := runPNG(t, "hello\nworld\n")
	_, zeroPadPNG := runPNG(t, "hello\nworld\n", "--padding", "0")
	if !bytes.Equal(faithfulPNG, zeroPadPNG) {
		t.Fatalf("--padding 0 PNG differs from Faithful PNG")
	}
}

func TestChromeSVGAllBuiltinThemes(t *testing.T) {
	// Default Faithful mode has no Chrome.
	plainSVG, stderr, code := run(t, "hello world\n")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(plainSVG, `width="99" height="18"`) {
		t.Fatalf("expected Faithful height 18 without --chrome, got:\n%s", plainSVG)
	}
	if strings.Contains(plainSVG, "#ff5f56") || strings.Contains(plainSVG, "<circle") {
		t.Fatalf("expected no Chrome elements without --chrome:\n%s", plainSVG)
	}

	themes := []struct {
		name      string
		chromeBg  string
		buttons   [3]string
		wantShape string
		wantLeft  bool
	}{
		{
			name:      "dark",
			chromeBg:  "#2d2f31",
			buttons:   [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
			wantShape: `<circle cx="16" cy="16" r="6" fill="#ff5f56"/>`,
			wantLeft:  true,
		},
		{
			name:      "light",
			chromeBg:  "#e8e8e8",
			buttons:   [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
			wantShape: `<circle cx="16" cy="16" r="6" fill="#ff5f56"/>`,
			wantLeft:  true,
		},
		{
			name:      "macos",
			chromeBg:  "#e5e5e5",
			buttons:   [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
			wantShape: `<circle cx="16" cy="16" r="6" fill="#ff5f56"/>`,
			wantLeft:  true,
		},
		{
			name:      "iterm2",
			chromeBg:  "#323232",
			buttons:   [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
			wantShape: `<circle cx="16" cy="16" r="6" fill="#ff5f56"/>`,
			wantLeft:  true,
		},
		{
			name:      "gnome",
			chromeBg:  "#303030",
			buttons:   [3]string{"#4a4a4a", "#4a4a4a", "#cc0000"},
			wantShape: `<circle cx="83" cy="16" r="6" fill="#cc0000"/>`,
			wantLeft:  false,
		},
		{
			name:      "windows",
			chromeBg:  "#1f1f1f",
			buttons:   [3]string{"#cccccc", "#cccccc", "#e81123"},
			wantShape: `fill="#e81123"`,
			wantLeft:  false,
		},
		{
			name:      "dracula",
			chromeBg:  "#21222c",
			buttons:   [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
			wantShape: `<circle cx="16" cy="16" r="6" fill="#ff5f56"/>`,
			wantLeft:  true,
		},
		{
			name:      "catppuccin-mocha",
			chromeBg:  "#181825",
			buttons:   [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
			wantShape: `<circle cx="16" cy="16" r="6" fill="#ff5f56"/>`,
			wantLeft:  true,
		},
		{
			name:      "catppuccin-latte",
			chromeBg:  "#e6e9ef",
			buttons:   [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
			wantShape: `<circle cx="16" cy="16" r="6" fill="#ff5f56"/>`,
			wantLeft:  true,
		},
		{
			name:      "tokyo-night",
			chromeBg:  "#16161e",
			buttons:   [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
			wantShape: `<circle cx="16" cy="16" r="6" fill="#ff5f56"/>`,
			wantLeft:  true,
		},
		{
			name:      "nord",
			chromeBg:  "#242933",
			buttons:   [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
			wantShape: `<circle cx="16" cy="16" r="6" fill="#ff5f56"/>`,
			wantLeft:  true,
		},
		{
			name:      "gruvbox-dark",
			chromeBg:  "#1d2021",
			buttons:   [3]string{"#ff5f56", "#ffbd2e", "#27c93f"},
			wantShape: `<circle cx="16" cy="16" r="6" fill="#ff5f56"/>`,
			wantLeft:  true,
		},
	}

	for _, tc := range themes {
		t.Run(tc.name, func(t *testing.T) {
			out, stderr, code := run(t, "hello world\n", "--theme", tc.name, "--chrome")
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
			}
			if !strings.Contains(out, `width="99" height="50"`) {
				t.Fatalf("theme %s: expected width=99 height=50 with --chrome:\n%s", tc.name, out)
			}
			wantBar := `height="32" fill="` + tc.chromeBg + `"`
			if !strings.Contains(out, wantBar) {
				t.Errorf("theme %s: missing title bar %s:\n%s", tc.name, wantBar, out)
			}
			for _, btn := range tc.buttons {
				if !strings.Contains(out, btn) {
					t.Errorf("theme %s: missing button color %s:\n%s", tc.name, btn, out)
				}
			}
			if !strings.Contains(out, tc.wantShape) {
				t.Errorf("theme %s: missing expected button shape %s:\n%s", tc.name, tc.wantShape, out)
			}
			if got := svgTexts(t, out); !slices.Equal(got, []string{"hello world"}) {
				t.Errorf("theme %s: texts = %q, want [hello world]", tc.name, got)
			}
			if !strings.Contains(out, `<text x="0" y="45"`) {
				t.Errorf("theme %s: expected text shifted down to y=45:\n%s", tc.name, out)
			}
		})
	}
}

func TestChromePNGAllBuiltinThemes(t *testing.T) {
	themes := []struct {
		name     string
		termBg   color.NRGBA
		chromeBg color.NRGBA
		buttons  [3]color.NRGBA
		left     bool
	}{
		{
			name:     "dark",
			termBg:   color.NRGBA{0x1d, 0x1f, 0x21, 0xff},
			chromeBg: color.NRGBA{0x2d, 0x2f, 0x31, 0xff},
			buttons:  [3]color.NRGBA{{0xff, 0x5f, 0x56, 0xff}, {0xff, 0xbd, 0x2e, 0xff}, {0x27, 0xc9, 0x3f, 0xff}},
			left:     true,
		},
		{
			name:     "light",
			termBg:   color.NRGBA{0xff, 0xff, 0xff, 0xff},
			chromeBg: color.NRGBA{0xe8, 0xe8, 0xe8, 0xff},
			buttons:  [3]color.NRGBA{{0xff, 0x5f, 0x56, 0xff}, {0xff, 0xbd, 0x2e, 0xff}, {0x27, 0xc9, 0x3f, 0xff}},
			left:     true,
		},
		{
			name:     "macos",
			termBg:   color.NRGBA{0xff, 0xff, 0xff, 0xff},
			chromeBg: color.NRGBA{0xe5, 0xe5, 0xe5, 0xff},
			buttons:  [3]color.NRGBA{{0xff, 0x5f, 0x56, 0xff}, {0xff, 0xbd, 0x2e, 0xff}, {0x27, 0xc9, 0x3f, 0xff}},
			left:     true,
		},
		{
			name:     "iterm2",
			termBg:   color.NRGBA{0x00, 0x00, 0x00, 0xff},
			chromeBg: color.NRGBA{0x32, 0x32, 0x32, 0xff},
			buttons:  [3]color.NRGBA{{0xff, 0x5f, 0x56, 0xff}, {0xff, 0xbd, 0x2e, 0xff}, {0x27, 0xc9, 0x3f, 0xff}},
			left:     true,
		},
		{
			name:     "gnome",
			termBg:   color.NRGBA{0x2e, 0x34, 0x36, 0xff},
			chromeBg: color.NRGBA{0x30, 0x30, 0x30, 0xff},
			buttons:  [3]color.NRGBA{{0x4a, 0x4a, 0x4a, 0xff}, {0x4a, 0x4a, 0x4a, 0xff}, {0xcc, 0x00, 0x00, 0xff}},
			left:     false,
		},
		{
			name:     "windows",
			termBg:   color.NRGBA{0x0c, 0x0c, 0x0c, 0xff},
			chromeBg: color.NRGBA{0x1f, 0x1f, 0x1f, 0xff},
			buttons:  [3]color.NRGBA{{0xcc, 0xcc, 0xcc, 0xff}, {0xcc, 0xcc, 0xcc, 0xff}, {0xe8, 0x11, 0x23, 0xff}},
			left:     false,
		},
		{
			name:     "dracula",
			termBg:   color.NRGBA{0x28, 0x2a, 0x36, 0xff},
			chromeBg: color.NRGBA{0x21, 0x22, 0x2c, 0xff},
			buttons:  [3]color.NRGBA{{0xff, 0x5f, 0x56, 0xff}, {0xff, 0xbd, 0x2e, 0xff}, {0x27, 0xc9, 0x3f, 0xff}},
			left:     true,
		},
		{
			name:     "catppuccin-mocha",
			termBg:   color.NRGBA{0x1e, 0x1e, 0x2e, 0xff},
			chromeBg: color.NRGBA{0x18, 0x18, 0x25, 0xff},
			buttons:  [3]color.NRGBA{{0xff, 0x5f, 0x56, 0xff}, {0xff, 0xbd, 0x2e, 0xff}, {0x27, 0xc9, 0x3f, 0xff}},
			left:     true,
		},
		{
			name:     "catppuccin-latte",
			termBg:   color.NRGBA{0xef, 0xf1, 0xf5, 0xff},
			chromeBg: color.NRGBA{0xe6, 0xe9, 0xef, 0xff},
			buttons:  [3]color.NRGBA{{0xff, 0x5f, 0x56, 0xff}, {0xff, 0xbd, 0x2e, 0xff}, {0x27, 0xc9, 0x3f, 0xff}},
			left:     true,
		},
		{
			name:     "tokyo-night",
			termBg:   color.NRGBA{0x1a, 0x1b, 0x26, 0xff},
			chromeBg: color.NRGBA{0x16, 0x16, 0x1e, 0xff},
			buttons:  [3]color.NRGBA{{0xff, 0x5f, 0x56, 0xff}, {0xff, 0xbd, 0x2e, 0xff}, {0x27, 0xc9, 0x3f, 0xff}},
			left:     true,
		},
		{
			name:     "nord",
			termBg:   color.NRGBA{0x2e, 0x34, 0x40, 0xff},
			chromeBg: color.NRGBA{0x24, 0x29, 0x33, 0xff},
			buttons:  [3]color.NRGBA{{0xff, 0x5f, 0x56, 0xff}, {0xff, 0xbd, 0x2e, 0xff}, {0x27, 0xc9, 0x3f, 0xff}},
			left:     true,
		},
		{
			name:     "gruvbox-dark",
			termBg:   color.NRGBA{0x28, 0x28, 0x28, 0xff},
			chromeBg: color.NRGBA{0x1d, 0x20, 0x21, 0xff},
			buttons:  [3]color.NRGBA{{0xff, 0x5f, 0x56, 0xff}, {0xff, 0xbd, 0x2e, 0xff}, {0x27, 0xc9, 0x3f, 0xff}},
			left:     true,
		},
	}

	for _, tc := range themes {
		t.Run(tc.name, func(t *testing.T) {
			img, _ := runPNG(t, "hello world\n", "--theme", tc.name, "--chrome")
			if got := img.Bounds().Size(); got != image.Pt(99, 50) {
				t.Fatalf("theme %s: size = %v, want (99, 50)", tc.name, got)
			}
			if got := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA); got != tc.chromeBg {
				t.Errorf("theme %s: title bar pixel (0,0) = %#v, want %#v", tc.name, got, tc.chromeBg)
			}
			if got := color.NRGBAModel.Convert(img.At(0, 32)).(color.NRGBA); got != tc.termBg {
				t.Errorf("theme %s: content pixel (0,32) = %#v, want %#v", tc.name, got, tc.termBg)
			}
			barColors := make(map[color.NRGBA]int)
			xStart, xEnd := 0, 68
			if !tc.left {
				xStart, xEnd = 35, 99
			}
			for y := 0; y < 32; y++ {
				for x := xStart; x < xEnd; x++ {
					c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
					barColors[c]++
				}
			}
			for _, btn := range tc.buttons {
				if barColors[btn] == 0 {
					t.Errorf("theme %s: button color %#v not found in title bar buttons region [%d..%d)", tc.name, btn, xStart, xEnd)
				}
			}
		})
	}
}

func TestShadowAndChromeCombinedSVGAndPNG(t *testing.T) {
	svgShadow, stderr, code := run(t, "hello world\n", "--shadow")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(svgShadow, `width="139" height="58"`) {
		t.Fatalf("expected SVG size 139x58 with --shadow, got:\n%s", svgShadow)
	}
	if !strings.Contains(svgShadow, `<filter id="shadow"`) {
		t.Errorf("expected SVG shadow filter with --shadow:\n%s", svgShadow)
	}
	if !strings.Contains(svgShadow, `<text x="20" y="33"`) {
		t.Errorf("expected text shifted by shadowPad (20, 20) to x=20 y=33:\n%s", svgShadow)
	}
	if got := svgTexts(t, svgShadow); !slices.Equal(got, []string{"hello world"}) {
		t.Errorf("texts = %q, want [hello world]", got)
	}

	svgBoth, stderr, code := run(t, "hello world\n", "--chrome", "--shadow")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(svgBoth, `width="139" height="90"`) {
		t.Fatalf("expected SVG size 139x90 with --chrome --shadow, got:\n%s", svgBoth)
	}
	if !strings.Contains(svgBoth, `<filter id="shadow"`) || !strings.Contains(svgBoth, `#ff5f56`) {
		t.Errorf("expected both shadow filter and chrome buttons in SVG:\n%s", svgBoth)
	}
	if !strings.Contains(svgBoth, `<text x="20" y="65"`) {
		t.Errorf("expected text at x=20 y=65 (20 + 32 + 13):\n%s", svgBoth)
	}

	pngShadow, _ := runPNG(t, "hello world\n", "--shadow")
	if got := pngShadow.Bounds().Size(); got != image.Pt(139, 58) {
		t.Fatalf("PNG --shadow size = %v, want (139, 58)", got)
	}
	if corner := color.NRGBAModel.Convert(pngShadow.At(0, 0)).(color.NRGBA); corner.A != 0 {
		t.Errorf("PNG --shadow outer corner (0,0) alpha = %d, want 0", corner.A)
	}
	belowWin := color.NRGBAModel.Convert(pngShadow.At(50, 20+18+2)).(color.NRGBA)
	if belowWin.A == 0 || belowWin.A == 0xff {
		t.Errorf("PNG --shadow pixel below window (50,40) = %#v, want translucent shadow (0 < A < 255)", belowWin)
	}
	insideWin := color.NRGBAModel.Convert(pngShadow.At(20, 20)).(color.NRGBA)
	if insideWin != (color.NRGBA{0x1d, 0x1f, 0x21, 0xff}) {
		t.Errorf("PNG --shadow window pixel (20,20) = %#v, want #1d1f21", insideWin)
	}

	pngBoth, _ := runPNG(t, "hello world\n", "--chrome", "--shadow")
	if got := pngBoth.Bounds().Size(); got != image.Pt(139, 90) {
		t.Fatalf("PNG --chrome --shadow size = %v, want (139, 90)", got)
	}
	if got := color.NRGBAModel.Convert(pngBoth.At(20, 20)).(color.NRGBA); got != (color.NRGBA{0x2d, 0x2f, 0x31, 0xff}) {
		t.Errorf("PNG --chrome --shadow chrome pixel (20,20) = %#v, want #2d2f31", got)
	}
	if got := color.NRGBAModel.Convert(pngBoth.At(20, 52)).(color.NRGBA); got != (color.NRGBA{0x1d, 0x1f, 0x21, 0xff}) {
		t.Errorf("PNG --chrome --shadow content pixel (20,52) = %#v, want #1d1f21", got)
	}
	belowBoth := color.NRGBAModel.Convert(pngBoth.At(50, 20+50+2)).(color.NRGBA)
	if belowBoth.A == 0 || belowBoth.A == 0xff {
		t.Errorf("PNG --chrome --shadow pixel below window (50,72) = %#v, want translucent shadow", belowBoth)
	}

	// All Enhancements combined (--trim --padding 8 --chrome --shadow).
	svgAll, stderr, code := run(t, "\n  hi  \n", "--cols", "8", "--rows", "4", "--trim", "--padding", "8", "--chrome", "--shadow")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	// Trimmed grid is 2x1 (18x18) + 2*8 padding = 34x34 + 32 chromeH = 34x66 + 2*20 shadowPad = 74x106.
	if !strings.Contains(svgAll, `width="74" height="106"`) {
		t.Fatalf("expected combined SVG size 74x106, got:\n%s", svgAll)
	}
	pngAll, _ := runPNG(t, "\n  hi  \n", "--cols", "8", "--rows", "4", "--trim", "--padding", "8", "--chrome", "--shadow")
	if got := pngAll.Bounds().Size(); got != image.Pt(74, 106) {
		t.Fatalf("combined PNG size = %v, want (74, 106)", got)
	}
}

func TestChromeWindowTitleSVGAndPNG(t *testing.T) {
	// Non-empty --title automatically enables Chrome without requiring --chrome.
	// 30 cols = 270 px wide. Safe zone = 270 - 144 = 126 px (14 cells).
	// Title "git status" is 10 cols (90 px), centered at x = (270 - 90)/2 = 90, y = 21.
	svgOut, stderr, code := run(t, "hello\n", "--cols", "30", "--title", "git status")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if !strings.Contains(svgOut, `width="270" height="50"`) {
		t.Fatalf("expected --title to auto-enable Chrome (height 50), got:\n%s", svgOut)
	}
	if got := svgTexts(t, svgOut); !slices.Equal(got, []string{"git status", "hello"}) {
		t.Fatalf("texts = %q, want [\"git status\" \"hello\"]", got)
	}
	if !strings.Contains(svgOut, `<text x="90" y="21" fill="#c5c8c6" textLength="90" lengthAdjust="spacing">git status</text>`) {
		t.Fatalf("missing centered Chrome title text element:\n%s", svgOut)
	}

	// Empty --title "" without --chrome keeps Chrome disabled.
	emptyTitleSVG, stderr, code := run(t, "hello\n", "--title", "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if !strings.Contains(emptyTitleSVG, `width="45" height="18"`) {
		t.Fatalf("expected empty --title without --chrome to keep Chrome off (45x18), got:\n%s", emptyTitleSVG)
	}

	// Built-in Themes use chrome.foreground for the title in SVG and PNG.
	themeFgCases := []struct {
		name   string
		wantFg string
		wantC  color.NRGBA
	}{
		{name: "dark", wantFg: "#c5c8c6", wantC: color.NRGBA{0xc5, 0xc8, 0xc6, 0xff}},
		{name: "light", wantFg: "#4d4d4c", wantC: color.NRGBA{0x4d, 0x4d, 0x4c, 0xff}},
		{name: "macos", wantFg: "#333333", wantC: color.NRGBA{0x33, 0x33, 0x33, 0xff}},
		{name: "iterm2", wantFg: "#c7c7c7", wantC: color.NRGBA{0xc7, 0xc7, 0xc7, 0xff}},
		{name: "gnome", wantFg: "#eeeeec", wantC: color.NRGBA{0xee, 0xee, 0xec, 0xff}},
		{name: "windows", wantFg: "#cccccc", wantC: color.NRGBA{0xcc, 0xcc, 0xcc, 0xff}},
	}
	for _, tc := range themeFgCases {
		out, stderr, code := run(t, "hello\n", "--cols", "30", "--theme", tc.name, "--title", "M")
		if code != 0 {
			t.Fatalf("theme %s: exit code = %d (stderr: %q)", tc.name, code, stderr)
		}
		if !strings.Contains(out, `fill="`+tc.wantFg+`" textLength="9" lengthAdjust="spacing">M</text>`) {
			t.Fatalf("theme %s: expected title fill %s:\n%s", tc.name, tc.wantFg, out)
		}
		img, _ := runPNG(t, "hello\n", "--cols", "30", "--theme", tc.name, "--title", "MMM")
		if got := img.Bounds().Size(); got != image.Pt(270, 50) {
			t.Fatalf("theme %s PNG size = %v, want (270, 50)", tc.name, got)
		}
		// Sample center of title bar (x in [120..150), y in [8..24)) and verify title foreground pixels are drawn.
		foundFg := false
		for y := 8; y < 24; y++ {
			for x := 120; x < 150; x++ {
				if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA) == tc.wantC {
					foundFg = true
					break
				}
			}
		}
		if !foundFg {
			t.Fatalf("theme %s PNG: title foreground color %#v not found in center of title bar", tc.name, tc.wantC)
		}
	}

	// Narrow Screenshot truncates long title with "…" within the safe zone between Chrome buttons
	// without widening the Screenshot.
	// 20 cols = 180 px wide. Safe zone = 180 - 144 = 36 px = 4 cells.
	// Title "hello world" (11 cols) truncates to "hel…" (4 cols = 36 px), centered at x = (180 - 36)/2 = 72.
	narrowSVG, stderr, code := run(t, "hi\n", "--cols", "20", "--title", "hello world")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if !strings.Contains(narrowSVG, `width="180" height="50"`) {
		t.Fatalf("expected narrow Screenshot width 180 unchanged, got:\n%s", narrowSVG)
	}
	if got := svgTexts(t, narrowSVG); !slices.Equal(got, []string{"hel…", "hi"}) {
		t.Fatalf("narrow texts = %q, want [\"hel…\" \"hi\"]", got)
	}
	if !strings.Contains(narrowSVG, `<text x="72" y="21" fill="#c5c8c6" textLength="36" lengthAdjust="spacing">hel…</text>`) {
		t.Fatalf("expected truncated title at x=72 textLength=36:\n%s", narrowSVG)
	}

	// Wide CJK characters are not split when truncating with "…":
	// Safe zone = 4 cells (20 cols = 180 px). Title "日本語" (6 cols) -> "日…" (3 cols = 27 px) since 2nd wide char wouldn't fit with "…".
	cjkSVG, stderr, code := run(t, "hi\n", "--cols", "20", "--title", "日本語")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if got := svgTexts(t, cjkSVG); !slices.Equal(got, []string{"日…", "hi"}) {
		t.Fatalf("CJK truncated texts = %q, want [\"日…\" \"hi\"]", got)
	}

	// Very narrow Screenshot (safe zone < 1 cell, e.g. 10 cols = 90 px -> safeW = 90 - 144 < 9) omits title cleanly.
	tinySVG, stderr, code := run(t, "hi\n", "--cols", "10", "--title", "hello")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr)
	}
	if !strings.Contains(tinySVG, `width="90" height="50"`) {
		t.Fatalf("expected tiny Screenshot 90x50 with Chrome enabled, got:\n%s", tinySVG)
	}
	if got := svgTexts(t, tinySVG); !slices.Equal(got, []string{"hi"}) {
		t.Fatalf("tiny texts = %q, want [\"hi\"] (title omitted)", got)
	}

	// Custom JSON Theme without chrome.foreground falls back to theme.foreground.
	customTheme := `{
		"foreground": "#123456",
		"background": "#000000",
		"ansi": [
			"#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000",
			"#000000","#000000","#000000","#000000","#000000","#000000","#000000","#000000"
		],
		"chrome": {"background": "#222222"}
	}`
	themePath := filepath.Join(t.TempDir(), "custom.json")
	if err := os.WriteFile(themePath, []byte(customTheme), 0o644); err != nil {
		t.Fatal(err)
	}
	customOut, stderr, code := run(t, "hi\n", "--cols", "30", "--theme", themePath, "--title", "custom")
	if code != 0 {
		t.Fatalf("custom theme exit code = %d (stderr: %q)", code, stderr)
	}
	if !strings.Contains(customOut, `fill="#123456" textLength="54" lengthAdjust="spacing">custom</text>`) {
		t.Fatalf("expected custom theme title to fall back to theme.foreground #123456:\n%s", customOut)
	}
}

func TestUserConfigDefaultsAndCLIOverridePrecedence(t *testing.T) {
	// 1. Missing default config file in $XDG_CONFIG_HOME/ssterm/config.json is silently ignored.
	emptyXDG := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", emptyXDG)
	t.Setenv("SSTERM_CONFIG", "")
	out, stderr, code := run(t, "hello\n")
	if code != 0 || stderr != "" {
		t.Fatalf("missing default config: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(out, `width="45" height="18"`) || !strings.Contains(out, `fill="#1d1f21"`) {
		t.Fatalf("expected Faithful dark defaults when config.json does not exist:\n%s", out)
	}

	// 2. Loading defaults from $XDG_CONFIG_HOME/ssterm/config.json applies theme, font, padding, trim, chrome, shadow.
	cfgDir := filepath.Join(emptyXDG, "ssterm")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "config.json")
	cfgJSON := `{
		"theme": "macos",
		"font": "JetBrains Mono, monospace",
		"padding": 10,
		"trim": true,
		"chrome": true,
		"shadow": true
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	out, stderr, code = run(t, "\n  hi  \n", "--cols", "8", "--rows", "4")
	if code != 0 {
		t.Fatalf("config defaults: code=%d stderr=%q", code, stderr)
	}
	// Trimmed "hi" is 2x1 (18x18) + 2*10 padding = 38x38 + 32 Chrome = 38x70 + 2*20 shadow = 78x110.
	for _, want := range []string{
		`width="78" height="110"`,
		`fill="#e5e5e5"`,
		`fill="#ffffff"`,
		`<filter id="shadow"`,
		`font-family="JetBrains Mono, monospace"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("config defaults SVG missing %s:\n%s", want, out)
		}
	}

	// 3. Explicit CLI flags (including boolean false and padding 0) override config.json.
	overrideOut, stderr, code := run(t, "\n  hi  \n", "--cols", "8", "--rows", "4",
		"--theme", "dark",
		"--font", "Fira Code",
		"--padding", "0",
		"--trim=false",
		"--chrome=false",
		"--shadow=false",
	)
	if code != 0 {
		t.Fatalf("CLI override: code=%d stderr=%q", code, stderr)
	}
	// Untrimmed 8x4 = 72x72, no padding, no Chrome, no shadow, dark theme (#1d1f21), Fira Code font.
	for _, want := range []string{
		`width="72" height="72"`,
		`<rect width="72" height="72" fill="#1d1f21"/>`,
		`font-family="Fira Code"`,
	} {
		if !strings.Contains(overrideOut, want) {
			t.Fatalf("CLI override SVG missing %s:\n%s", want, overrideOut)
		}
	}
	if strings.Contains(overrideOut, `<filter id="shadow"`) || strings.Contains(overrideOut, `#e5e5e5`) {
		t.Fatalf("CLI override did not disable shadow or macos Chrome:\n%s", overrideOut)
	}

	// 4. $SSTERM_CONFIG overrides $XDG_CONFIG_HOME/ssterm/config.json and supports "shell": true.
	customCfg := filepath.Join(t.TempDir(), "my-config.json")
	if err := os.WriteFile(customCfg, []byte(`{"theme": "gnome", "padding": 4, "shell": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSTERM_CONFIG", customCfg)
	out, stderr, code = run(t, "hi\n")
	if code != 0 {
		t.Fatalf("SSTERM_CONFIG: code=%d stderr=%q", code, stderr)
	}
	// "hi" is 2x1 (18x18) + 2*4 padding = 26x26, gnome background #2e3436.
	if !strings.Contains(out, `width="26" height="26"`) || !strings.Contains(out, `fill="#2e3436"`) {
		t.Fatalf("expected SSTERM_CONFIG gnome theme with padding 4 (26x26):\n%s", out)
	}
	if runtime.GOOS != "windows" {
		if bashPath, err := exec.LookPath("bash"); err == nil {
			home := t.TempDir()
			if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte("alias cfg_alias='printf \"from-cfg-shell\\n\"'\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", home)
			t.Setenv("SHELL", bashPath)
			ptyOut, stderr, code := run(t, "", "--", "cfg_alias")
			if code != 0 {
				t.Fatalf("config shell=true PTY: code=%d stderr=%q", code, stderr)
			}
			if got := svgTexts(t, ptyOut); !slices.Equal(got, []string{"from-cfg-shell"}) {
				t.Fatalf("config shell=true texts = %q, want [from-cfg-shell]", got)
			}
		}
	}
}

func TestUserConfigValidationErrors(t *testing.T) {
	// Missing explicit $SSTERM_CONFIG file fails with exit code 2.
	missingPath := filepath.Join(t.TempDir(), "does-not-exist.json")
	t.Setenv("SSTERM_CONFIG", missingPath)
	out, stderr, code := run(t, "hi\n")
	if code != 2 || out != "" || !strings.Contains(stderr, missingPath) {
		t.Fatalf("missing SSTERM_CONFIG: code=%d out=%q stderr=%q", code, out, stderr)
	}

	cases := []struct {
		name      string
		body      string
		wantError string
	}{
		{
			name:      "malformed JSON",
			body:      `{"theme":`,
			wantError: "invalid JSON",
		},
		{
			name:      "top-level null",
			body:      `null`,
			wantError: "expected object",
		},
		{
			name:      "unknown field",
			body:      `{"theme": "dark", "unknown_flag": true}`,
			wantError: "unknown_flag",
		},
		{
			name:      "null theme",
			body:      `{"theme": null}`,
			wantError: "theme",
		},
		{
			name:      "wrong type padding",
			body:      `{"padding": "12"}`,
			wantError: "padding",
		},
		{
			name:      "float padding",
			body:      `{"padding": 1.5}`,
			wantError: "padding",
		},
		{
			name:      "negative padding",
			body:      `{"padding": -4}`,
			wantError: "--padding",
		},
		{
			name:      "wrong type chrome",
			body:      `{"chrome": "yes"}`,
			wantError: "chrome",
		},
		{
			name:      "unknown theme value",
			body:      `{"theme": "no-such-theme"}`,
			wantError: "no-such-theme",
		},
	}

	dir := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "_")+".json")
			if err := os.WriteFile(p, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SSTERM_CONFIG", p)
			out, stderr, code := run(t, "hi\n")
			if code != 2 {
				t.Fatalf("%s: exit code = %d, want 2 (stderr: %q)", tc.name, code, stderr)
			}
			if out != "" {
				t.Fatalf("%s: expected empty stdout, got %q", tc.name, out)
			}
			if !strings.Contains(stderr, tc.wantError) {
				t.Fatalf("%s: stderr %q does not contain %q", tc.name, stderr, tc.wantError)
			}
		})
	}
}
