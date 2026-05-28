package testutils

import "net/http"

//go:generate mockgen -destination=mocks/mock_flusher.go -package=mocks . ResponseWriterFlusher

type ResponseWriterFlusher interface {
	http.ResponseWriter
	http.Flusher
}
