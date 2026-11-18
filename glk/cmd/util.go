package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// AskForConfirm reads a yes/no answer from stdin. An empty answer, EOF or a
// read error counts as "no".
func AskForConfirm() bool {
	return askForConfirm(bufio.NewScanner(os.Stdin))
}

func askForConfirm(in *bufio.Scanner) bool {
	for in.Scan() {
		switch strings.ToUpper(strings.TrimSpace(in.Text())) {
		case "Y", "YES":
			return true
		case "", "N", "NO":
			return false
		}
		fmt.Print("Please type [y/n]: ")
	}
	return false
}
