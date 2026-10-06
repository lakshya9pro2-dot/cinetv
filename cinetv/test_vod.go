//go:build ignore

package main

import (
	"fmt"
	"encoding/json"
)

func main() {
    // 1. Search for Fight Club 1999
    res := apiPost("/api/search/result", map[string]string{"kw": "Fight Club 1999", "pn": "1"})
    fmt.Printf("Search result: %+v\n", res)
    
    // We would extract the ID and call getVodInfo
}
