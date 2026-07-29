package aliyunopenapimeta

import (
	"encoding/json"
	"io/fs"
	"testing"
)

func TestPackedMetadatasReadCanonicalAPI(t *testing.T) {
	data, err := fs.ReadFile(Metadatas, "canonical/ecs/2014-05-26/DescribeRegions.json")
	if err != nil {
		t.Fatalf("ReadFile canonical API: %v", err)
	}
	var api struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &api); err != nil {
		t.Fatalf("Unmarshal canonical API: %v", err)
	}
	if api.Name != "DescribeRegions" {
		t.Fatalf("api name = %q, want DescribeRegions", api.Name)
	}
}

func TestPackedMetadatasReadDirAndStat(t *testing.T) {
	entries, err := fs.ReadDir(Metadatas, "canonical/ecs/2014-05-26")
	if err != nil {
		t.Fatalf("ReadDir canonical version: %v", err)
	}
	found := false
	for _, entry := range entries {
		if entry.Name() == "DescribeRegions.json" {
			found = true
			if entry.IsDir() {
				t.Fatal("DescribeRegions.json reported as directory")
			}
			break
		}
	}
	if !found {
		t.Fatal("DescribeRegions.json not found in canonical version directory")
	}

	info, err := fs.Stat(Metadatas, "canonical/ecs")
	if err != nil {
		t.Fatalf("Stat canonical product: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("canonical/ecs reported as file")
	}
}

func TestPackedMetadatasReadProducts(t *testing.T) {
	data, err := fs.ReadFile(Metadatas, "metadatas/products.json")
	if err != nil {
		t.Fatalf("ReadFile products: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("products.json is empty")
	}
}
