// Package api holds the OpenAPI description of Melon's REST API.
package api

import _ "embed"

//go:embed openapi.yaml
var OpenAPI []byte
