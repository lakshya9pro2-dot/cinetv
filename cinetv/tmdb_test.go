//go:build ignore

package main

import (
	"fmt"
	"net/http"
	"io/ioutil"
)

func main() {
    resp, _ := http.Get("http://localhost:1937/api/tv/6678/1/6")
    body, _ := ioutil.ReadAll(resp.Body)
    fmt.Println(string(body))
}
