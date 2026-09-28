package storage

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"ninimenu/internal/config"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func OwnerFromKey(key string) uint {
	if p := config.C.S3Prefix; p != "" {
		key = strings.TrimPrefix(key, p+"/")
	}
	parts := strings.Split(key, "/")
	if len(parts) < 3 || parts[0] != "u" {
		return 0
	}
	n, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0
	}
	return uint(n)
}

func DeleteBackup(key string) error {
	if config.C.BackupDir == "" {
		return nil
	}
	if !filepath.IsLocal(key) {
		return errors.New("invalid backup key")
	}
	err := os.Remove(filepath.Join(config.C.BackupDir, filepath.FromSlash(key)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func walkLocal(ctx context.Context, root string, visit func(string, int64) error) error {
	if root == "" {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) && path == root {
			return nil
		}
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		key, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return visit(filepath.ToSlash(key), info.Size())
	})
}

func WalkBackups(ctx context.Context, visit func(string, int64) error) error {
	return walkLocal(ctx, config.C.BackupDir, visit)
}

// WalkUploads paginates S3 inventory; no response or object list grows with the
// bucket. Local historical files are included even after switching to S3.
func WalkUploads(ctx context.Context, visit func(string, int64) error) error {
	if err := walkLocal(ctx, config.C.UploadDir, visit); err != nil {
		return err
	}
	s := s3Client()
	if s == nil {
		return nil
	}
	cursor := ""
	for {
		u, err := s.objectURL("")
		if err != nil {
			return err
		}
		q := u.Query()
		q.Set("list-type", "2")
		q.Set("max-keys", "500")
		prefix := "u/"
		if config.C.S3Prefix != "" {
			prefix = config.C.S3Prefix + "/u/"
		}
		q.Set("prefix", prefix)
		if cursor != "" {
			q.Set("continuation-token", cursor)
		}
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return err
		}
		s.sign(req, nil)
		client := s.HTTP
		if client == nil {
			client = http.DefaultClient
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 {
			err := s3Error(resp)
			resp.Body.Close()
			return err
		}
		var page struct {
			Contents []struct {
				Key  string
				Size int64
			}
			IsTruncated           bool
			NextContinuationToken string
		}
		err = xml.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return err
		}
		for _, object := range page.Contents {
			if !filepath.IsLocal(object.Key) || object.Size < 0 {
				return errors.New("invalid inventory object")
			}
			// A historical key can exist in both stores. The second callback
			// replaces its ledger total with the sum, rather than charging once.
			if info, err := os.Stat(filepath.Join(config.C.UploadDir, filepath.FromSlash(object.Key))); err == nil && info.Mode().IsRegular() {
				object.Size += info.Size()
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := visit(object.Key, object.Size); err != nil {
				return err
			}
		}
		if !page.IsTruncated {
			return nil
		}
		if page.NextContinuationToken == "" || page.NextContinuationToken == cursor {
			return errors.New("invalid inventory cursor")
		}
		cursor = page.NextContinuationToken
	}
}
