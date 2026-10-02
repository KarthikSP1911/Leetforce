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
	sum := 0
	for i := 0; i < n; i++ {
		var x int
		fmt.Fscan(r, &x)
		sum += x
	}
	fmt.Println(sum)
}
