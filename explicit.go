//go:build cgo

package gorattler

import "fmt"

// ExplicitSpec is a parsed @EXPLICIT environment file.
type ExplicitSpec struct {
	Platform string
	Packages []ExplicitPackage
}

// ExplicitPackage is one explicit package URL with its hash separated out.
// URL has its fragment removed; callers should redact credentials before logging.
type ExplicitPackage struct {
	URL    string
	SHA256 string
	MD5    string
}

// ParseExplicitSpec parses an @EXPLICIT document. Entries keep install order.
func ParseExplicitSpec(doc []byte) (ExplicitSpec, error) {
	var out struct {
		Platform *string `json:"platform"`
		Packages []struct {
			URL    string  `json:"url"`
			MD5    *string `json:"md5"`
			SHA256 *string `json:"sha256"`
		} `json:"packages"`
	}
	if err := callJSON(explicitOperation, struct {
		Document string `json:"document"`
	}{encodeDocument(doc)}, &out); err != nil {
		return ExplicitSpec{}, fmt.Errorf("parse explicit spec: %w", err)
	}
	spec := ExplicitSpec{Packages: make([]ExplicitPackage, 0, len(out.Packages))}
	if out.Platform != nil {
		spec.Platform = *out.Platform
	}
	for _, p := range out.Packages {
		pkg := ExplicitPackage{URL: p.URL}
		if p.MD5 != nil {
			pkg.MD5 = *p.MD5
		}
		if p.SHA256 != nil {
			pkg.SHA256 = *p.SHA256
		}
		spec.Packages = append(spec.Packages, pkg)
	}
	return spec, nil
}
