// internal/batch/loader_test.go
package batch_test

import (
	"os"
	"testing"

	"github.com/pablofelix/acm-caas-poc/internal/batch"
)

func writeTmp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(content)
	f.Close()
	return f.Name()
}

func TestLoadFilePlainList(t *testing.T) {
	path := writeTmp(t, "clusters:\n  - spoke1\n  - spoke2\n")
	items, err := batch.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "spoke1" {
		t.Errorf("unexpected name: %s", items[0].Name)
	}
}

func TestLoadFileObjectList(t *testing.T) {
	path := writeTmp(t, "clusters:\n  - name: spoke1\n    platform: ibmcloud\n    region: us-south\n")
	items, err := batch.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Platform != "ibmcloud" {
		t.Errorf("unexpected platform: %s", items[0].Platform)
	}
}

func TestLoadFileEmptyError(t *testing.T) {
	path := writeTmp(t, "clusters: []\n")
	_, err := batch.LoadFile(path)
	if err == nil {
		t.Error("expected error for empty list")
	}
}

func TestNamesFromArgsDuplicateError(t *testing.T) {
	args := []string{"spoke1"}
	fileItems := []batch.ClusterItem{{Name: "spoke1"}}
	_, err := batch.NamesFromArgs(args, fileItems)
	if err == nil {
		t.Error("expected duplicate error")
	}
}

func TestNamesFromArgsMerge(t *testing.T) {
	args := []string{"a"}
	fileItems := []batch.ClusterItem{{Name: "b"}}
	items, err := batch.NamesFromArgs(args, fileItems)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "a" || items[1].Name != "b" {
		t.Errorf("unexpected order: %+v", items)
	}
}

func TestNamesFromArgsNoArgsNoFile(t *testing.T) {
	_, err := batch.NamesFromArgs(nil, nil)
	if err == nil {
		t.Error("expected error when neither args nor file provided")
	}
}
