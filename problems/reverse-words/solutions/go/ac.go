package main

import (
    "bufio"
    "fmt"
    "os"
    "strings"
)

func main() {
    r := bufio.NewReader(os.Stdin)
    var n int
    fmt.Fscan(r, &n)
    w := make([]string, n)
    for i := range w {
        fmt.Fscan(r, &w[i])
    }
    for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
        w[i], w[j] = w[j], w[i]
    }
    fmt.Println(strings.Join(w, " "))
}
