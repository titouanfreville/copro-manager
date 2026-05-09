// visiondump runs Cloud Vision DOCUMENT_TEXT_DETECTION against one or
// more local image files and dumps everything our OCR pipeline sees:
//
//  1. The raw word-level blocks Vision returned (text + normalized
//     bounding box).
//  2. The fragments after our number-extraction pass.
//  3. The final candidates the pipeline would feed into the
//     pick-best / blue-anchor stages.
//
// Authentication is via Application Default Credentials — the same
// path Cloud Run uses. Run from the api/ directory:
//
//	go run ./bin/visiondump /path/to/photo1.jpg /path/to/photo2.jpg
//
// Or against a directory:
//
//	go run ./bin/visiondump /path/to/folder/
//
// Output is JSON to stdout — pipe to `jq` for readability.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
	"github.com/titouanfreville/copro-manager/api/src/services/vision"
)

type photoDump struct {
	Path       string                    `json:"path"`
	Blocks     []interfaces.OCRTextBlock `json:"blocks"`
	BlockCount int                       `json:"block_count"`
	Error      string                    `json:"error,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: visiondump <file-or-dir> [file ...]")
		os.Exit(2)
	}

	files := []string{}
	for _, arg := range os.Args[1:] {
		info, err := os.Stat(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat %q: %v\n", arg, err)
			continue
		}
		if info.IsDir() {
			entries, err := os.ReadDir(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "readdir %q: %v\n", arg, err)
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := strings.ToLower(e.Name())
				if !strings.HasSuffix(name, ".jpg") &&
					!strings.HasSuffix(name, ".jpeg") &&
					!strings.HasSuffix(name, ".png") {
					continue
				}
				files = append(files, filepath.Join(arg, e.Name()))
			}
		} else {
			files = append(files, arg)
		}
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no image files found")
		os.Exit(2)
	}

	// Operator dump tool — bypass the per-month cap (nil usage store) and
	// run with `enabled: true` regardless of the production config.
	client, err := vision.NewClient(vision.Config{Enabled: true}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vision client: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	out := make([]photoDump, 0, len(files))
	for _, f := range files {
		bytes, err := os.ReadFile(f)
		if err != nil {
			out = append(out, photoDump{Path: f, Error: err.Error()})
			continue
		}
		blocks, err := client.DetectTextFromBytes(ctx, bytes)
		if err != nil {
			out = append(out, photoDump{Path: f, Error: err.Error()})
			continue
		}
		out = append(out, photoDump{
			Path:       f,
			Blocks:     blocks,
			BlockCount: len(blocks),
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
}
