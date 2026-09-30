package httpapi

import (
	"compress/gzip"
	"io"
	"net/http"
)

type gzipWriter struct {
	http.ResponseWriter
	zw *gzip.Writer
}

func newGzipWriter(w http.ResponseWriter) *gzipWriter {
	return &gzipWriter{ResponseWriter: w, zw: gzip.NewWriter(w)}
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	return g.zw.Write(b)
}

// Flush pushes compressed bytes and is required for streaming responses.
func (g *gzipWriter) Flush() {
	_ = g.zw.Flush()
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (g *gzipWriter) Close() error { return g.zw.Close() }

var _ io.Closer = (*gzipWriter)(nil)
