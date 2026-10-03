package main

import (
    "bufio"
    "fmt"
    "os"
)

func main() {
    r := bufio.NewReader(os.Stdin)
    var n int
    fmt.Fscan(r, &n)
    var best, cur int64
    fmt.Fscan(r, &best)
    cur = best
    for i := 1; i < n; i++ {
        var x int64
        fmt.Fscan(r, &x)
        if cur+x > x {
            cur += x
        } else {
            cur = x
        }
        if cur > best {
            best = cur
        }
    }
    fmt.Println(best)
}
