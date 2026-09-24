package storage

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// AWS 官方文档 “Signature Calculations for the Authorization Header” 中 GET Object 的示例。
func TestSignV4MatchesAWSExample(t *testing.T) {
	s := &S3{
		Endpoint:  "https://s3.amazonaws.com",
		Region:    "us-east-1",
		Bucket:    "examplebucket",
		AccessKey: "AKIAIOSFODNN7EXAMPLE",
		SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		PathStyle: false,
		now:       func() time.Time { return time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC) },
	}
	u, err := s.objectURL("test.txt")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	req.Header.Set("Range", "bytes=0-9")
	s.sign(req, nil)

	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("authorization mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestPathStyleURL(t *testing.T) {
	s := &S3{Endpoint: "http://127.0.0.1:3900", Bucket: "cook-uploads", PathStyle: true}
	u, err := s.objectURL("/cook/u/1/2026/09/24/a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if got := u.String(); got != "http://127.0.0.1:3900/cook-uploads/cook/u/1/2026/09/24/a.jpg" {
		t.Fatalf("unexpected url %s", got)
	}
}

func TestURIEncode(t *testing.T) {
	if got := uriEncode("/a b/c~d", false); got != "/a%20b/c~d" {
		t.Fatalf("got %s", got)
	}
	if got := uriEncode("a/b", true); !strings.Contains(got, "%2F") {
		t.Fatalf("slash should be encoded: %s", got)
	}
}
