// gameir-validate validates one declarative Game IR document.
package main

import (
	"flag"
	"fmt"
	"os"

	"doodlequest/internal/gameir"
)

func main() {
	file := flag.String("file", "", "path to a Game IR JSON file")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "usage: gameir-validate -file path/to/game-ir.json")
		os.Exit(2)
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read Game IR: %v\n", err)
		os.Exit(1)
	}
	doc, err := gameir.DecodeAndValidate(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid Game IR: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("valid Game IR v%s: %s (%d scenes, %d assets, %d rules)\n",
		doc.IRVersion, doc.Game.ID, len(doc.Scenes), len(doc.Assets), len(doc.Rules))
}
