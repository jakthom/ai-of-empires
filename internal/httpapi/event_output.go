package httpapi

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Compress each authenticated stream separately and flush every frame. This
// reduces large-map bandwidth without changing the versioned SSE payloads or
// buffering movement updates until a compression block fills.
type eventOutput struct {
	io.Writer
	zip        *gzip.Writer
	controller *http.ResponseController
}

func newEventOutput(w http.ResponseWriter, r *http.Request) *eventOutput {
	o := &eventOutput{Writer: w, controller: http.NewResponseController(w)}
	w.Header().Add("Vary", "Accept-Encoding")
	if acceptsGzip(r.Header.Get("Accept-Encoding")) {
		o.zip, _ = gzip.NewWriterLevel(w, gzip.BestSpeed)
		o.Writer = o.zip
		w.Header().Set("Content-Encoding", "gzip")
	}
	return o
}

func acceptsGzip(header string) bool {
	wildcard := false
	for _, item := range strings.Split(header, ",") {
		parts := strings.Split(item, ";")
		name := strings.TrimSpace(parts[0])
		if !strings.EqualFold(name, "gzip") && name != "*" {
			continue
		}
		quality := 1.
		for _, param := range parts[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
			if ok && strings.EqualFold(key, "q") {
				n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
				if err != nil || n < 0 || n > 1 {
					quality = 0
				} else {
					quality = n
				}
			}
		}
		if strings.EqualFold(name, "gzip") {
			return quality > 0
		}
		wildcard = quality > 0
	}
	return wildcard
}

func (o *eventOutput) Flush() error {
	if o.zip != nil {
		if err := o.zip.Flush(); err != nil {
			return err
		}
	}
	return o.controller.Flush()
}

func (o *eventOutput) Close() {
	if o.zip != nil {
		_ = o.zip.Close()
	}
}
