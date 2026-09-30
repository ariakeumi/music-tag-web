package scraper

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"time"
)

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func jsonUnmarshal(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	return dec.Decode(v)
}

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func readFileWD(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func timeNow() time.Time { return time.Now() }
