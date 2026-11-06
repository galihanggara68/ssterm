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
	assertTexts(t, "ab\x1b[2J\x1b[Hz", []string{"ab", "z"}, "--cols", "4", "--rows", "2")
	assertTexts(t, "ab\x1b[H\x1b[0J\x1b[3Jz", []string{"z"}, "--cols", "4", "--rows", "2")
}

func TestScrollbackIncludedByDefault(t *testing.T) {
	assertTexts(t, "1\n2\n3\n4\n5\n", []string{"1", "2", "3", "4", "5"}, "--rows", "2")
}

func TestAlternateScreenYieldsFinalGrid(t *testing.T) {
	assertTexts(t, "main\n\x1b[?1049h\x1b[Halt\x1b[2;1Hscreen", []string{"alt", "screen"}, "--rows", "2")
	assertTexts(t, "main\n\x1b[?1049h\x1b[Halt\x1b[?1049l", []string{"main"}, "--rows", "2")
}

func TestGoldenCursorOutput(t *testing.T) {
	golden(t, "progress", "Downloading\n 10%\r 55%\r100%\nstatus: ok\x1b[3D\x1b[Kdone\n1\n2\n3\n", "--rows", "4")
}
