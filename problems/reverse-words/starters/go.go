package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func solve(words []string) []string {
	// return the words in reverse order
	return words
}

func main() {
	r := bufio.NewReader(os.Stdin)
	var n int
	fmt.Fscan(r, &n)
	words := make([]string, n)
	for i := range words {
		fmt.Fscan(r, &words[i])
	}
	fmt.Println(strings.Join(solve(words), " "))
}
