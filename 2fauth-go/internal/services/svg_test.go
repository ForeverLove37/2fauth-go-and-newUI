package services

import (
	"strings"
	"testing"
)

func TestValidIconName(t *testing.T) {
	for _, name := range []string{"abc123.png", "a.b.svg", "icon_1-2.webp", "NoExtension"} {
		if !ValidIconName(name) {
			t.Errorf("%q refused", name)
		}
	}
	for _, name := range []string{"", ".", "..", "../x", "a/b", `a\b`, ".hidden", "a#b", "a?b", "a b", strings.Repeat("x", 300)} {
		if ValidIconName(name) {
			t.Errorf("%q accepted", name)
		}
	}
}

func TestSanitizeSVGKeepsARealIcon(t *testing.T) {
	icon := `<?xml version="1.0"?><!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 24 24" width="24" height="24">
  <defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#fff"/></linearGradient></defs>
  <title>Logo</title><!-- a comment -->
  <g fill="url(#g)" transform="scale(2)"><path d="M0 0h24v24H0z"/><use xlink:href="#g"/></g>
</svg>`
	out, err := SanitizeSVG([]byte(icon))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`viewBox="0 0 24 24"`, `<linearGradient`, `fill="url(#g)"`, `d="M0 0h24v24H0z"`, `href="#g"`, `xmlns="http://www.w3.org/2000/svg"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
	if _, err := SanitizeSVG(out); err != nil {
		t.Errorf("the output is not stable: %v", err)
	}
}

func TestSanitizeSVGRefusals(t *testing.T) {
	for _, doc := range []string{
		"", "plain text", "<html><svg></svg></html>", "<svg>", "<svg></svg><svg></svg>", "<svg></svg>trailing",
		"leading<svg></svg>", `<svg><a>&undefined;</a></svg>`,
	} {
		if _, err := SanitizeSVG([]byte(doc)); err == nil {
			t.Errorf("%q accepted", doc)
		}
	}
}

func TestSanitizeSVGBoundsItsWork(t *testing.T) {
	deep := strings.Repeat("<g>", 100) + strings.Repeat("</g>", 100)
	if _, err := SanitizeSVG([]byte("<svg>" + deep + "</svg>")); err == nil {
		t.Error("a very deep document was accepted")
	}
}
