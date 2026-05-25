// Package apispec embeds the OpenAPI specification into the binary so it can
// be served at GET /openapi.yaml without additional runtime file dependencies.
package apispec

import _ "embed"

//go:embed openapi.yaml
var Spec []byte
