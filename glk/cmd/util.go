package cmd

import (
	"fmt"
	"os"
	"strings"
)

func AskForConfirm() bool {
	var input string
	_, err := fmt.Scanln(&input)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(255)
	}
	if input == "y" || input == "Y" || strings.ToUpper(input) == "YES" {
		return true
	}
	if input == "n" || input == "N" || strings.ToUpper(input) == "NO" {
		return false
	}
	fmt.Println("Please type [y/n]: ")
	return AskForConfirm()
}
