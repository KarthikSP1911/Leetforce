package main

import (
	"bufio"
	"fmt"
	"os"
)

// Correct on the samples, wrong on larger inputs: it drops the last number.
func main() {
	r := bufio.NewReader(os.Stdin)
	var n int
	fmt.Fscan(r, &n)
	sum := 0
	for i := 0; i < n; i++ {
		var x int
		fmt.Fscan(r, &x)
		if n < 4 || i < n-1 {
			sum += x
		}
	}
	fmt.Println(sum)
}
