package services

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"
)

// svgMime is the content type of a sanitized SVG.
const svgMime = "image/svg+xml"

// iconNamePattern is the only shape of an icon file name: what the service generates
// (a content hash and an extension) and the file names of upstream databases. No
// separator and no leading dot means it can never leave the icons directory, and
// never reach anything else on the origin when used as a URL.
var iconNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*(\.[A-Za-z0-9]+)*$`)

// ValidIconName reports whether name can designate a stored icon.
func ValidIconName(name string) bool {
	return len(name) <= 255 && iconNamePattern.MatchString(name)
}

// svgElements and svgAttributes are the only parts of an SVG document that are kept.
// It is a list of what is known to be harmless rather than of what is known to be
// dangerous: scripts, event handlers, foreignObject, external references and
// animations of those simply have no way through.
var svgElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "title": true, "desc": true, "symbol": true, "use": true,
	"path": true, "rect": true, "circle": true, "ellipse": true, "line": true, "polyline": true, "polygon": true,
	"text": true, "tspan": true, "lineargradient": true, "radialgradient": true, "stop": true,
	"clippath": true, "mask": true, "pattern": true, "filter": true, "fegaussianblur": true, "feoffset": true,
	"feblend": true, "fecolormatrix": true, "fecomposite": true, "feflood": true, "femerge": true, "femergenode": true,
}

var svgAttributes = map[string]bool{
	"id": true, "class": true, "version": true, "x": true, "y": true, "x1": true, "y1": true, "x2": true, "y2": true,
	"cx": true, "cy": true, "r": true, "rx": true, "ry": true, "fx": true, "fy": true, "d": true, "points": true,
	"width": true, "height": true, "viewbox": true, "preserveaspectratio": true, "transform": true,
	"fill": true, "fill-opacity": true, "fill-rule": true, "stroke": true, "stroke-width": true, "stroke-opacity": true,
	"stroke-linecap": true, "stroke-linejoin": true, "stroke-miterlimit": true, "stroke-dasharray": true,
	"stroke-dashoffset": true, "opacity": true, "clip-path": true, "clip-rule": true, "mask": true, "filter": true,
	"offset": true, "stop-color": true, "stop-opacity": true, "gradientunits": true, "gradienttransform": true,
	"spreadmethod": true, "patternunits": true, "patterntransform": true, "patterncontentunits": true,
	"clippathunits": true, "maskunits": true, "maskcontentunits": true, "font-family": true, "font-size": true,
	"font-weight": true, "text-anchor": true, "dx": true, "dy": true, "href": true, "xmlns": true,
	"stddeviation": true, "in": true, "in2": true, "result": true, "mode": true, "type": true, "values": true,
	"flood-color": true, "flood-opacity": true, "operator": true, "filterunits": true, "color": true,
	"display": true, "visibility": true, "style": true,
}

// localPaintReference matches url(#id), the only url() form an icon may use.
var localPaintReference = regexp.MustCompile(`(?i)^\s*url\(\s*["']?#[A-Za-z0-9_.:-]+["']?\s*\)\s*$`)

// errNotSVG reports bytes that are not a clean SVG document.
var errNotSVG = errors.New("not an SVG document")

// SanitizeSVG parses an SVG and writes it back with allowed elements and attributes
// only. The result is a new document: nothing of the input survives unparsed.
func SanitizeSVG(content []byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.Strict = true
	decoder.Entity = map[string]string{}

	var out bytes.Buffer
	encoder := xml.NewEncoder(&out)
	depth, skipDepth, elements := 0, 0, 0
	sawRoot := false
	closed := false

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errNotSVG
		}
		switch t := token.(type) {
		case xml.StartElement:
			elements++
			if elements > 20000 || depth > 64 {
				return nil, errNotSVG
			}
			name := strings.ToLower(t.Name.Local)
			if !sawRoot {
				if name != "svg" {
					return nil, errNotSVG
				}
				sawRoot = true
			} else if closed {
				return nil, errNotSVG
			}
			depth++
			if skipDepth > 0 {
				continue
			}
			if !svgElements[name] {
				skipDepth = depth
				continue
			}
			start := xml.StartElement{Name: xml.Name{Local: t.Name.Local}, Attr: cleanSVGAttrs(t.Attr, depth == 1)}
			if err := encoder.EncodeToken(start); err != nil {
				return nil, errNotSVG
			}
		case xml.EndElement:
			depth--
			if skipDepth > 0 {
				if depth < skipDepth {
					skipDepth = 0
				}
				continue
			}
			if err := encoder.EncodeToken(xml.EndElement{Name: xml.Name{Local: t.Name.Local}}); err != nil {
				return nil, errNotSVG
			}
			if depth == 0 {
				closed = true
			}
		case xml.CharData:
			if skipDepth > 0 || depth == 0 {
				if depth == 0 && len(bytes.TrimSpace(t)) > 0 {
					return nil, errNotSVG
				}
				continue
			}
			if err := encoder.EncodeToken(t); err != nil {
				return nil, errNotSVG
			}
		}
		// Comments, processing instructions and the DOCTYPE (the way to declare
		// entities) fall through: they are simply not copied.
	}
	if !sawRoot || !closed {
		return nil, errNotSVG
	}
	if err := encoder.Flush(); err != nil {
		return nil, errNotSVG
	}
	return out.Bytes(), nil
}

func cleanSVGAttrs(attrs []xml.Attr, root bool) []xml.Attr {
	out := make([]xml.Attr, 0, len(attrs)+1)
	if root {
		out = append(out, xml.Attr{Name: xml.Name{Local: "xmlns"}, Value: "http://www.w3.org/2000/svg"})
	}
	for _, attr := range attrs {
		name := strings.ToLower(attr.Name.Local)
		if attr.Name.Space == "xmlns" || name == "xmlns" || !svgAttributes[name] {
			continue
		}
		value := attr.Value
		if name == "href" {
			if !strings.HasPrefix(strings.TrimSpace(value), "#") {
				continue
			}
		} else if strings.Contains(strings.ToLower(value), "url(") && !localPaintReference.MatchString(value) {
			continue
		}
		out = append(out, xml.Attr{Name: xml.Name{Local: attr.Name.Local}, Value: value})
	}
	return out
}
