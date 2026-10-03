package main

import "fmt"

func main() {
    var s string
    fmt.Scan(&s)
    var st []byte
    ok := true
    for i := 0; i < len(s) && ok; i++ {
        ch := s[i]
        if ch == '(' || ch == '[' {
            st = append(st, ch)
            continue
        }
        want := byte('[')
        if ch == ')' {
            want = '('
        }
        if len(st) == 0 || st[len(st)-1] != want {
            ok = false
        } else {
            st = st[:len(st)-1]
        }
    }
    if ok && len(st) == 0 {
        fmt.Println("YES")
    } else {
        fmt.Println("NO")
    }
}
