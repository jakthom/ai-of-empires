package httpapi

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEventCompressionFlushesEachFrame(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(fmt.Sprint(compressed), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			next := make(chan struct{})
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				o := newEventOutput(w, r)
				defer o.Close()
				w.Header().Set("Content-Type", "text/event-stream")
				for i := 0; i < 3; i++ {
					fmt.Fprintf(o, "data: %d\n\n", i)
					if o.Flush() != nil {
						return
					}
					select {
					case <-next:
					case <-r.Context().Done():
						return
					}
				}
			}))
			defer s.Close()
			req, _ := http.NewRequestWithContext(ctx, "GET", s.URL, nil)
			req.Header.Set("Accept-Encoding", "identity")
			if compressed {
				req.Header.Set("Accept-Encoding", "gzip")
			}
			response, err := s.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var scanner *bufio.Scanner
			if compressed {
				if response.Header.Get("Content-Encoding") != "gzip" {
					t.Fatal("missing negotiated compression")
				}
				z, err := gzip.NewReader(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				defer z.Close()
				scanner = bufio.NewScanner(z)
			} else {
				if response.Header.Get("Content-Encoding") != "" {
					t.Fatal("compressed an identity request")
				}
				scanner = bufio.NewScanner(response.Body)
			}
			for i := 0; i < 3; i++ {
				if !scanner.Scan() || scanner.Text() != fmt.Sprintf("data: %d", i) {
					t.Fatalf("frame %d was buffered or corrupted: %s %v", i, scanner.Text(), scanner.Err())
				}
				if !scanner.Scan() || scanner.Text() != "" {
					t.Fatal("missing SSE boundary")
				}
				next <- struct{}{}
			}
			if scanner.Scan() || scanner.Err() != nil {
				t.Fatal("invalid stream footer", scanner.Err())
			}
		})
	}
}

func TestEventCompressionNegotiation(t *testing.T) {
	for _, c := range []struct {
		header string
		want   bool
	}{
		{"", false}, {"identity", false}, {"br, gzip, deflate", true},
		{"gzip;q=0", false}, {"gzip; q=0.5", true}, {"*;q=1, gzip;q=0", false},
		{"*;q=0.5", true}, {"gzip;q=invalid", false}, {"gzip;q=2", false},
	} {
		if got := acceptsGzip(c.header); got != c.want {
			t.Errorf("%q: got %v, want %v", c.header, got, c.want)
		}
	}
}
