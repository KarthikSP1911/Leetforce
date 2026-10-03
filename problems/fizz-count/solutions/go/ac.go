package main

import "fmt"

func main() {
    var n int64
    fmt.Scan(&n)
    fmt.Println(n/3 + n/5 - 2*(n/15))
}
