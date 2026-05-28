package models

type CacheData struct {
	Status  int
	Body    []byte
	Headers map[string][]string
}
