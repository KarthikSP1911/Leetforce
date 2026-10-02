package main

import (
	"fmt"
	"strings"
)

func main() {
	line := strings.Repeat("x", 1000)
	for {
		fmt.Println(line)
	}
}
