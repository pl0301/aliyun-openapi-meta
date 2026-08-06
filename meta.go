package aliyunopenapimeta

import (
	"embed"
)

//go:embed canonical
//go:embed metadatas/products.json
var Metadatas embed.FS
