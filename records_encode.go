//go:build cgo

package gorattler

import "encoding/base64"

// encodeDocument encodes raw document bytes for the JSON bridge. The Rust side
// decodes them with crate::bridge::decode_document.
func encodeDocument(doc []byte) string {
	return base64.StdEncoding.EncodeToString(doc)
}
