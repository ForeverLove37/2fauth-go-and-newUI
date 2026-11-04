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
	"fecomponenttransfer": true, "fefunca": true, "fefuncr": true, "fefuncg": true, "fefuncb": true, "fedropshadow": true,
	"femorphology": true, "marker": true, "switch": true, "style": true, "image": true,
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
	"display": true, "visibility": true, "style": true, "font-style": true, "letter-spacing": true, "paint-order": true,
	"vector-effect": true, "transform-origin": true, "mix-blend-mode": true, "isolation": true, "shape-rendering": true,
	"color-interpolation-filters": true, "primitiveunits": true, "tablevalues": true, "slope": true, "intercept": true,
	"exponent": true, "amplitude": true, "radius": true, "markerwidth": true, "markerheight": true, "orient": true,
	"refx": true, "refy": true, "mask-type": true, "image-rendering": true, "dominant-baseline": true, "text-rendering": true,
}

// localPaintReference matches url(#id), the only url() form an icon may use.
var localPaintReference = regexp.MustCompile(`(?i)^\s*url\(\s*["']?#[A-Za-z0-9_.:-]+["']?\s*\)\s*$`)

// embeddedRaster matches the only image an icon may embed: a raster in a data URI. An
// external file would be a request, and an embedded SVG a way around this list.
var embeddedRaster = regexp.MustCompile(`(?i)^\s*data:image/(png|jpe?g|gif|webp);base64,[A-Za-z0-9+/=\s]+$`)

// hasEmbeddedRaster reports whether an image element holds its picture itself.
func hasEmbeddedRaster(attrs []xml.Attr) bool {
	for _, attr := range attrs {
		if strings.EqualFold(attr.Name.Local, "href") && embeddedRaster.MatchString(attr.Value) {
			return true
		}
	}
	return false
}

// safeCSS reports whether a style sheet or a style attribute cannot load anything.
// Backslashes are refused because CSS escapes could spell url() in other ways.
func safeCSS(css string) bool {
	lower := strings.ToLower(css)
	if strings.ContainsAny(lower, "\\<") || strings.Contains(lower, "@import") || strings.Contains(lower, "expression(") ||
		strings.Contains(lower, "javascript:") {
		return false
	}
	for rest := lower; ; {
		i := strings.Index(rest, "url(")
		if i < 0 {
			return true
		}
		rest = strings.TrimLeft(rest[i+4:], " \t\r\n\"'")
		if !strings.HasPrefix(rest, "#") {
			return false
		}
	}
}

// Namespaces of the parts of a document that are kept.
const (
	svgNamespace   = "http://www.w3.org/2000/svg"
	xlinkNamespace = "http://www.w3.org/1999/xlink"
)

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
	// The content of a style element is checked as a whole, once it is complete.
	inStyle := false
	var css bytes.Buffer

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
			// Editors add elements of their own, some named like real ones.
			foreign := t.Name.Space != "" && t.Name.Space != svgNamespace
			if foreign || !svgElements[name] || (name == "image" && !hasEmbeddedRaster(t.Attr)) {
				skipDepth = depth
				continue
			}
			if name == "style" {
				inStyle = true
				css.Reset()
			}
			start := xml.StartElement{Name: xml.Name{Local: t.Name.Local}, Attr: cleanSVGAttrs(name, t.Attr, depth == 1)}
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
			if inStyle {
				inStyle = false
				if safeCSS(css.String()) {
					if err := encoder.EncodeToken(xml.CharData(css.Bytes())); err != nil {
						return nil, errNotSVG
					}
				}
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
			if inStyle {
				css.Write(t)
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

func cleanSVGAttrs(element string, attrs []xml.Attr, root bool) []xml.Attr {
	out := make([]xml.Attr, 0, len(attrs)+1)
	seen := map[string]bool{}
	if root {
		out = append(out, xml.Attr{Name: xml.Name{Local: "xmlns"}, Value: "http://www.w3.org/2000/svg"})
	}
	for _, attr := range attrs {
		name := strings.ToLower(attr.Name.Local)
		if attr.Name.Space == "xmlns" || name == "xmlns" || !svgAttributes[name] {
			continue
		}
		// inkscape:version is not version, and two attributes of the same name in the
		// output would make the whole document invalid. Only xlink:href is an alias.
		if attr.Name.Space != "" && !(attr.Name.Space == xlinkNamespace && name == "href") {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		value := attr.Value
		if name == "href" {
			local := strings.HasPrefix(strings.TrimSpace(value), "#")
			if element == "image" {
				local = embeddedRaster.MatchString(value)
			}
			if !local {
				continue
			}
		} else if name == "style" {
			if !safeCSS(value) {
				continue
			}
		} else if strings.Contains(strings.ToLower(value), "url(") && !localPaintReference.MatchString(value) {
			continue
		}
		out = append(out, xml.Attr{Name: xml.Name{Local: attr.Name.Local}, Value: value})
	}
	return out
}
