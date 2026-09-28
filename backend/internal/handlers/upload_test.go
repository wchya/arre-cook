package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type uploadPart struct {
	name, filename, data string
}

func uploadRequest(t testing.TB, parts ...uploadPart) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, part := range parts {
		var out io.Writer
		var err error
		if part.filename == "" {
			out, err = w.CreateFormField(part.name)
		} else {
			out, err = w.CreateFormFile(part.name, part.filename)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(out, part.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/upload/image", &buf)
	r.Header.Set("Content-Type", w.FormDataContentType())
	return r
}

func TestReadUploadImageBounds(t *testing.T) {
	imagePart := uploadPart{"image", "photo.png", strings.Repeat("x", 1024)}
	for _, chunked := range []bool{false, true} {
		for _, tc := range []struct {
			name  string
			parts []uploadPart
			valid bool
		}{
			{"exact file limit", []uploadPart{imagePart}, true},
			{"fields around file", []uploadPart{{"before", "", "value"}, imagePart, {"after", "", "value"}}, true},
			{"file over limit", []uploadPart{{"image", "photo.png", strings.Repeat("x", 1025)}}, false},
			{"fields over body limit", []uploadPart{imagePart, {"large", "", strings.Repeat("x", 65<<10)}}, false},
			{"missing file", []uploadPart{{"image", "", "text"}}, false},
			{"second file", []uploadPart{imagePart, imagePart}, false},
			{"empty first file", []uploadPart{{"image", "empty.png", ""}, imagePart}, false},
			{"wrong field", []uploadPart{{"other", "photo.png", "x"}}, false},
		} {
			t.Run(tc.name+"/chunked="+strconv.FormatBool(chunked), func(t *testing.T) {
				r := uploadRequest(t, tc.parts...)
				if chunked {
					r.ContentLength = -1
				}
				defer r.Body.Close()
				data, err := readUploadImage(httptest.NewRecorder(), r, 1024)
				if tc.valid {
					if err != nil || string(data) != imagePart.data {
						t.Fatalf("data length=%d err=%v", len(data), err)
					}
				} else if err == nil {
					t.Fatal("invalid or oversized upload accepted")
				}
			})
		}
	}
}

func TestReadUploadImageRejectsTruncatedMultipart(t *testing.T) {
	r := uploadRequest(t, uploadPart{"image", "photo.png", "image"})
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body[:len(body)-20]))
	r.ContentLength = -1
	if _, err := readUploadImage(httptest.NewRecorder(), r, 1024); err == nil {
		t.Fatal("truncated multipart accepted")
	}
}

type unreadUpload struct{ reads int }

func (r *unreadUpload) Read([]byte) (int, error) { r.reads++; return 0, io.EOF }
func (r *unreadUpload) Close() error             { return nil }

func TestUploadAdmissionDoesNotBufferQueuedBodies(t *testing.T) {
	if err := acquireImageUpload(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { <-imageUploadSlot }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := acquireImageUpload(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting upload ignored cancellation: %v", err)
	}
	if len(imageUploadWaiters) != 0 {
		t.Fatal("cancelled upload retained queue capacity")
	}
	for i := 0; i < cap(imageUploadWaiters); i++ {
		imageUploadWaiters <- struct{}{}
	}
	defer func() {
		for len(imageUploadWaiters) > 0 {
			<-imageUploadWaiters
		}
	}()
	body := &unreadUpload{}
	r := httptest.NewRequest(http.MethodPost, "/api/upload/image", body)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	UploadImage(c)
	if w.Code != http.StatusTooManyRequests || body.reads != 0 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("busy upload: status=%d reads=%d", w.Code, body.reads)
	}
}

func BenchmarkReadUploadImage(b *testing.B) {
	request := uploadRequest(b, uploadPart{"image", "photo.jpg", strings.Repeat("x", 5<<20)})
	body, _ := io.ReadAll(request.Body)
	for _, mode := range []string{"legacy_multipart", "streaming"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				r := httptest.NewRequest(http.MethodPost, "/api/upload/image", bytes.NewReader(body))
				r.Header.Set("Content-Type", request.Header.Get("Content-Type"))
				var data []byte
				var err error
				if mode == "streaming" {
					data, err = readUploadImage(httptest.NewRecorder(), r, 5<<20)
				} else {
					// Prior upload path, retained here solely as an allocation baseline.
					err = r.ParseMultipartForm(6 << 20)
					if err == nil {
						var file multipart.File
						file, _, err = r.FormFile("image")
						if err == nil {
							data, err = io.ReadAll(io.LimitReader(file, (5<<20)+1))
							file.Close()
						}
						r.MultipartForm.RemoveAll()
					}
				}
				r.Body.Close()
				if err != nil || len(data) != 5<<20 {
					b.Fatalf("length=%d err=%v", len(data), err)
				}
			}
		})
	}
}
