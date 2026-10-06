//go:build ignore

package main

import (
    "fmt"
    "net/http"
    "io/ioutil"
)

func main() {
    resp, _ := http.Get("http://localhost:1937/api/movie/550")
    body, _ := ioutil.ReadAll(resp.Body)
    fmt.Println(string(body))
}
