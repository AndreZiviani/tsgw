package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
)

type responseRecorder struct {
	w          http.ResponseWriter
	statusCode int
	bytes      int64
}

func (rw *responseRecorder) Header() http.Header { return rw.w.Header() }

func (rw *responseRecorder) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.w.WriteHeader(statusCode)
}

func (rw *responseRecorder) Write(p []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	n, err := rw.w.Write(p)
	rw.bytes += int64(n)
	return n, err
}

func (rw *responseRecorder) Flush() {
	if f, ok := rw.w.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.w.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return h.Hijack()
}

func (rw *responseRecorder) Push(target string, opts *http.PushOptions) error {
	if p, ok := rw.w.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}
