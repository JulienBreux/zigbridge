package webui_test

import (
	"io"
	"strings"
	"testing"

	"github.com/julienbreux/zigbridge/webui"
)

func TestDistFS(t *testing.T) {
	dist, err := webui.DistFS()
	if err != nil {
		t.Fatalf("DistFS() failed: %v", err)
	}

	f, err := dist.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open index.html: %v", err)
	}
	defer func() { _ = f.Close() }()

	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	if !strings.Contains(string(content), "Zigbridge") {
		t.Errorf("expected index.html to contain 'Zigbridge', got: %s", string(content))
	}
}
