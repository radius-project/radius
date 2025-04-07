package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	fluxSourceBranch = "release/v1.4.x"
)

// crds specifies the CRDs to download and their URLs.
// If we need to download more CRDs for testing, we can add them here.
//
// Here is the structure of the map:
//
//	crds = {
//		"nameofcrdfile.yaml": {
//			"pathofcrdfile": "urltofetchcrd"
//		}
//	}
var crds = map[string]map[string]string{
	"./flux": {
		"source.toolkit.fluxcd.io_gitrepositories.yaml": fmt.Sprintf("https://raw.githubusercontent.com/fluxcd/source-controller/%s/config/crd/bases/source.toolkit.fluxcd.io_gitrepositories.yaml", fluxSourceBranch),
	},
}

// PullCRDs downloads the CRDs from the upstream repository.
func PullCRDs(pwd string) {
	for directory, content := range crds {
		if _, err := os.Stat(filepath.Join(pwd, directory)); os.IsNotExist(err) {
			fmt.Printf("Directory %s does not exist. Please create it first.\n", directory)
			os.Exit(1) //nolint:forbidigo // this is OK inside of this tool, this is meant to be run from the command line
		}

		for fileName, url := range content {
			destFilePath := filepath.Join(pwd, directory, fileName)
			if err := downloadFile(url, destFilePath); err != nil {
				fmt.Printf("Error downloading %s: %v\n", fileName, err)
				os.Exit(1) //nolint:forbidigo // this is OK inside of this tool, this is meant to be run from the command line
			}
			fmt.Printf("Downloaded %s -> %s\n", url, destFilePath)
		}

		fmt.Println("Flux Source CRDs downloaded successfully.")
	}
}

// downloadFile fetches the remote URL and writes its contents to the specified filepath.
func downloadFile(url, filePath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, url)
	}

	outFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %q: %w", filePath, err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("failed to write to file %q: %w", filePath, err)
	}

	return nil
}

func main() {
	// PullCRDs is meant to be run from
	// the root of the project.
	PullCRDs("test/crd")
}
