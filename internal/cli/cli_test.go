package cli_test

import (
	"bytes"
	"encoding/xml"
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
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
	// Main-screen scrollback (1, 2, 3 on 2 rows) must not leak when the final
	// state is on the alternate screen.
	assertTexts(t, "1\n2\n3\n\x1b[?1049h\x1b[Halt\x1b[2;1Hscreen", []string{"alt", "screen"}, "--rows", "2")
	assertTexts(t, "main\n\x1b[?1049h\x1b[Halt\x1b[?1049l", []string{"main"}, "--rows", "2")
	// Auto-sized alternate screen addressing row 2 without newlines must not clip.
	assertTexts(t, "\x1b[?1049h\x1b[Htop\x1b[2;1Hbot", []string{"top", "bot"})
}

func TestAutoSizeFitsPostEmulationContent(t *testing.T) {
	// Erased tail should not inflate the auto-sized width beyond "status:done" (11 cols = 99px).
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
	// Wide chars and ASCII in the same row are anchored to their column x positions:
	// "日本語" at x=0 (width 54) and "x" at x=54 (width 9).
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
	// Corner pixel in cell (0,0) should be the dark theme background (#1d1f21).
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
		// Top-left corner of reversed cell (0,0) is cell background -> darkFg.
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


