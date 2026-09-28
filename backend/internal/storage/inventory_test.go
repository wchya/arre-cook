package storage

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"ninimenu/internal/config"
)

func inventoryServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	oldConfig, oldClient := config.C, s3Client()
	srv := httptest.NewServer(handler)
	t.Cleanup(func() { srv.Close(); config.C = oldConfig; client = oldClient })
	config.C.UploadDir = t.TempDir()
	config.C.S3Prefix = "cook"
	client = &S3{Endpoint: srv.URL, Bucket: "uploads", PathStyle: true, HTTP: srv.Client()}
}

func TestUploadInventoryPaginatesAndIncludesLocalFiles(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	inventoryServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		if r.URL.Query().Get("prefix") != "cook/u/" || r.URL.Query().Get("max-keys") != "500" {
			t.Error("inventory bounds missing")
		}
		if requests == 1 {
			fmt.Fprint(w, `<ListBucketResult><Contents><Key>cook/u/1/a.jpg</Key><Size>10</Size></Contents><IsTruncated>true</IsTruncated><NextContinuationToken>page2</NextContinuationToken></ListBucketResult>`)
		} else {
			if r.URL.Query().Get("continuation-token") != "page2" {
				t.Error("cursor missing")
			}
			fmt.Fprint(w, `<ListBucketResult><Contents><Key>cook/u/1/b.jpg</Key><Size>20</Size></Contents><IsTruncated>false</IsTruncated></ListBucketResult>`)
		}
	})
	if err := os.WriteFile(filepath.Join(config.C.UploadDir, "local.jpg"), []byte("local"), 0600); err != nil {
		t.Fatal(err)
	}
	duplicate := filepath.Join(config.C.UploadDir, "cook/u/1/a.jpg")
	if err := os.MkdirAll(filepath.Dir(duplicate), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(duplicate, []byte("copy"), 0600); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int64{}
	if err := WalkUploads(context.Background(), func(key string, size int64) error { seen[key] = size; return nil }); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 || seen["local.jpg"] != 5 || seen["cook/u/1/a.jpg"] != 14 || seen["cook/u/1/b.jpg"] != 20 {
		t.Fatalf("inventory=%v", seen)
	}
}

func TestUploadInventoryRejectsStalledCursor(t *testing.T) {
	inventoryServer(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>same</NextContinuationToken></ListBucketResult>`)
	})
	if err := WalkUploads(context.Background(), func(string, int64) error { return nil }); err == nil {
		t.Fatal("repeated cursor accepted")
	}
}

func TestDeleteRemovesLocalAndRemoteCopies(t *testing.T) {
	deleted := make(chan string, 1)
	inventoryServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Error("unexpected method")
		}
		deleted <- r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})
	file := filepath.Join(config.C.UploadDir, "photo.jpg")
	if err := os.WriteFile(file, []byte("photo"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Delete(context.Background(), "photo.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("local copy retained")
	}
	select {
	case path := <-deleted:
		if path != "/uploads/photo.jpg" {
			t.Fatalf("unexpected remote key %q", path)
		}
	default:
		t.Fatal("remote copy retained")
	}
}
