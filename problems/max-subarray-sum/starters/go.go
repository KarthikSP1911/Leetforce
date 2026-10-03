package main

import (
	"bufio"
	"fmt"
	"os"
)

func solve(a []int64) int64 {
	// return the largest sum of a non-empty contiguous subarray
	return 0
}

func main() {
	r := bufio.NewReader(os.Stdin)
	var n int
	fmt.Fscan(r, &n)
	a := make([]int64, n)
	for i := range a {
		fmt.Fscan(r, &a[i])
	}
	fmt.Println(solve(a))
}
