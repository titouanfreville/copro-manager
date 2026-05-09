//go:build realphotos

// Real-photo regression test. Calls Cloud Vision against the photos
// in `eau/` and runs them through analyzeImage end-to-end. Skipped
// by default since it costs Vision API quota; enable with:
//
//	cd api && go test -tags=realphotos ./src/domain/usecases/meters/... -v -run TestRealPhotos
//
// Authentication uses Application Default Credentials.
package meters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/services/vision"
)

// realPhotoCase ties a fixture file to its expected pipeline kind.
// `Want` is informational only — there's no automated assertion
// because we don't know the ground-truth value of every meter, just
// what behavior we want to observe.
type realPhotoCase struct {
	File string
	Kind entities.MeterPhotoKind
	Hint string // free-form note about what we expect / observed
}

func TestRealPhotos(t *testing.T) {
	root, err := findEauDir()
	if err != nil {
		t.Skipf("eau directory not found: %v", err)
	}
	cases := []realPhotoCase{
		{File: "Global_7_06_2025.jpg", Kind: entities.MeterPhotoKindGlobal,
			Hint: "rotated photo, large drum reading + chassis label 13MA / 103942 N"},
		{File: "compteur eau global 27 fev 2025.jpg", Kind: entities.MeterPhotoKindGlobal,
			Hint: "global meter Feb 27"},
		{File: "compteur eau global 8 avril 2025.jpg", Kind: entities.MeterPhotoKindGlobal,
			Hint: "global meter Apr 8"},
		{File: "compteur eau 8 avril 2025.jpg", Kind: entities.MeterPhotoKindDetail,
			Hint: "3-meter panel: 0007500 (blue/common), 00733695, 00739901"},
		{File: "compteur eau sep 27 fev 2025.jpg", Kind: entities.MeterPhotoKindDetail,
			Hint: "3-meter panel Feb 27"},
		{File: "detail_7_06_2025.jpg", Kind: entities.MeterPhotoKindDetail,
			Hint: "3-meter panel Jun 7 (rotated)"},
	}

	client, err := vision.NewClient()
	if err != nil {
		t.Fatalf("vision client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	uc := &usecases{
		logger: zap.NewNop(),
		ocr:    client,
		now:    time.Now,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, tc := range cases {
		path := filepath.Join(root, tc.File)
		bytes, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", tc.File, err)
			continue
		}
		res := uc.analyzeImage(ctx, tc.Kind, bytes)
		fmt.Printf("\n=== %s [%s] ===\n", tc.File, tc.Kind)
		fmt.Printf("hint: %s\n", tc.Hint)
		fmt.Printf("values: %v\n", res.Values)
		fmt.Printf("confidence: %v\n", res.Confidence)
	}
}

// findEauDir walks upward from the package's working directory until
// it locates a sibling `eau/` folder.
func findEauDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(cwd, "eau")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return "", fmt.Errorf("eau folder not found above %s", cwd)
		}
		cwd = parent
	}
}
