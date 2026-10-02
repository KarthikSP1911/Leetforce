package main

import "fmt"

func main() {
	block := make([]byte, 600<<20)
	for i := range block {
		block[i] = byte(i)
	}
	fmt.Println(len(block))
}
