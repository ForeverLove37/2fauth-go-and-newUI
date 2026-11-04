package services

import (
	"encoding/xml"
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

func TestSanitizeSVGKeepsStylesAndEmbeddedRasters(t *testing.T) {
	out, err := SanitizeSVG([]byte(`<svg viewBox="0 0 1 1" xmlns:xlink="http://www.w3.org/1999/xlink"><style><![CDATA[.a{fill:#f00}]]> .b{fill:url(#g)}</style><path class="a" d="M0 0"/><image xlink:href="data:image/png;base64,AAAA" width="1"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`.a{fill:#f00}`, `.b{fill:url(#g)}`, `class="a"`, `<image`, `href="data:image/png;base64,AAAA"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
}

func TestSanitizeSVGRefusesStylesThatLoadThings(t *testing.T) {
	for _, css := range []string{
		`@import "http://evil/x.css";`, `.a{background:url(http://evil/x)}`, `.a{background:url( 'https://evil')}`,
		`.a{background:\\75rl(http://evil)}`, `.a{width:expression(alert(1))}`, `.a{background:ur<![CDATA[l(http://evil)]]>}`,
	} {
		out, err := SanitizeSVG([]byte(`<svg><style>` + css + `</style></svg>`))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "evil") || strings.Contains(string(out), "expression") {
			t.Errorf("%s survived: %s", css, out)
		}
	}
	out, err := SanitizeSVG([]byte(`<svg><path style="fill:\\75rl(http://evil)" d="M0 0"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "evil") {
		t.Errorf("style attribute survived: %s", out)
	}
	for _, image := range []string{
		`<image href="http://evil/x.png"/>`, `<image href="data:image/svg+xml;base64,AAAA"/>`, `<image href="data:text/html;base64,AAAA"/>`, `<image/>`,
	} {
		out, err := SanitizeSVG([]byte(`<svg>` + image + `</svg>`))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "<image") {
			t.Errorf("%s survived: %s", image, out)
		}
	}
}

func TestSanitizeSVGDropsWhatEditorsAdd(t *testing.T) {
	icon := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd" xmlns:xlink="http://www.w3.org/1999/xlink" version="1.1" inkscape:version="1.4" viewBox="0 0 1 1">
	<sodipodi:namedview/><inkscape:path d="M9 9"/><path sodipodi:type="x" inkscape:label="y" d="M0 0"/><use href="#a" xlink:href="#b"/></svg>`
	out, err := SanitizeSVG([]byte(icon))
	if err != nil {
		t.Fatal(err)
	}
	var check struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(out, &check); err != nil {
		t.Fatalf("invalid output %s: %v", out, err)
	}
	if strings.Count(string(out), "version=") != 1 || !strings.Contains(string(out), `version="1.1"`) {
		t.Errorf("version: %s", out)
	}
	if strings.Contains(string(out), "M9 9") || strings.Contains(string(out), "namedview") || strings.Contains(string(out), `type="x"`) {
		t.Errorf("editor data kept: %s", out)
	}
	if strings.Count(string(out), "href=") != 1 {
		t.Errorf("href: %s", out)
	}
}
