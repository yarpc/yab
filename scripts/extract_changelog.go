package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("USAGE: %v VERSION\n", os.Args[0])
		os.Exit(1)
	}

	version := os.Args[1]
	if err := run(version, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

const (
	searching = iota
	foundHeader
	printing
)

func run(version string, out io.Writer) error {
	if version[0] == 'v' {
		version = version[1:]
	}

	file, err := os.OpenFile("CHANGELOG.md", os.O_RDONLY, 0444)
	if err != nil {
		return err
	}
	defer file.Close()

	state := searching
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		switch state {
		case searching:
			if line == "# "+version {
				state = foundHeader
			}
		case foundHeader:
			if line == "" {
				continue
			}
			state = printing
			fallthrough
		case printing:
			if strings.HasPrefix(line, "# ") {
				// next version section
				return nil
			}
			fmt.Fprintln(out, line)
		default:
			return fmt.Errorf("unexpected state %v on line %q", state, line)
		}

	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if state < printing {
		return fmt.Errorf("could not find version %q in changelog", version)
	}
	return nil
}
