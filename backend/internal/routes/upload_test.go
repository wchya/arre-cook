package routes_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"ninimenu/internal/config"
	"ninimenu/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadRejectsLargePixelsAndPreservesOriginalBackup(t *testing.T) {
	_, token, err := testutil.NewUser("memory-upload@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	var original bytes.Buffer
	if err := png.Encode(&original, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	upload := func(data []byte) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("image", "photo.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
		writer.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/upload/image", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	oversized := append([]byte(nil), original.Bytes()...)
	binary.BigEndian.PutUint32(oversized[16:20], 6000)
	binary.BigEndian.PutUint32(oversized[20:24], 4001)
	binary.BigEndian.PutUint32(oversized[29:33], crc32.ChecksumIEEE(oversized[12:29]))
	if w := upload(oversized); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "分辨率过高") {
		t.Fatalf("oversized pixels bypassed rejection: %d %s", w.Code, w.Body.String())
	}
	// A rejected image must release the processing slot, and valid uploads keep
	// the existing local backup and white-background JPEG behavior.
	w := upload(original.Bytes())
	if w.Code != http.StatusOK {
		t.Fatalf("valid upload failed: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Data struct{ URL string } `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	key := strings.TrimPrefix(response.Data.URL, "/uploads/")
	saved, err := os.ReadFile(filepath.Join(config.C.UploadDir, key))
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(saved))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(0, 0).RGBA()
	if r < 65000 || g < 65000 || b < 65000 {
		t.Fatalf("transparent PNG did not become white: %d %d %d", r, g, b)
	}
	backup, err := os.ReadFile(filepath.Join(config.C.BackupDir, strings.TrimSuffix(key, ".jpg")+".png"))
	if err != nil || !bytes.Equal(backup, original.Bytes()) {
		t.Fatalf("original backup changed: %v", err)
	}
}
