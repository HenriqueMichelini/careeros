// Command buildmanifest emits the owned co-build inputs for the local launcher.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	manifest, err := lab.CaptureBuildManifest(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Lab build manifest unavailable")
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(manifest); err != nil {
		os.Exit(1)
	}
}
