package diagnostics

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestYouTubeChecksAPIAndPayload(t *testing.T) {
	for _, mode := range []string{"good", "api-error", "truncated", "consent", "fake-image", "empty-contents"} {
		t.Run(mode, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
					if mode == "consent" {
						fmt.Fprint(w, "consent page")
					} else {
						fmt.Fprint(w, `{"INNERTUBE_CLIENT_VERSION":"2.20261108.00.00"}`)
					}
				case "/youtubei/v1/browse":
					if r.Method != "POST" {
						t.Error("API must be POST")
					}
					switch mode {
					case "api-error":
						fmt.Fprint(w, `{"error":{"code":403}}`)
					case "empty-contents":
						fmt.Fprint(w, `{"contents":null}`)
					case "truncated":
						fmt.Fprint(w, `{"contents":`)
					default:
						fmt.Fprint(w, `{"contents":{"twoColumnBrowseResultsRenderer":{}}}`)
					}
				default:
					if mode == "fake-image" {
						fmt.Fprint(w, "html")
					} else {
						jpeg.Encode(w, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil)
					}
				}
			}))
			defer s.Close()
			r := youtube(context.Background(), s.Client(), s.URL, s.URL)
			if r.OK != (mode == "good") {
				t.Fatalf("false success/failure: %+v", r)
			}
			if len(r.Checks) != 3 {
				t.Fatal(r)
			}
		})
	}
}
