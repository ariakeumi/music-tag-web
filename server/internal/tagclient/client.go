package tagclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

// Client drives py/tagcli.py, the Python side that owns all audio-tag
// reading/writing through component/music_tag. One short-lived process per
// call keeps state handling simple; batches go through a single invocation.
type Client struct {
	PythonBin  string
	ScriptPath string
}

func New(pythonBin, scriptPath string) *Client {
	return &Client{PythonBin: pythonBin, ScriptPath: scriptPath}
}

func (c *Client) run(args []string, stdin []byte, timeout time.Duration) ([]byte, error) {
	cmd := exec.Command(c.PythonBin, append([]string{c.ScriptPath}, args...)...)
	cmd.Stdin = bytes.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("python start: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("tagcli: %v: %s", err, errOut.String())
		}
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("tagcli timed out after %s", timeout)
	}
	return out.Bytes(), nil
}

func parseJSON(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("tagcli output: %w (%s)", err, truncate(data))
	}
	return nil
}

func truncate(b []byte) string {
	if len(b) > 300 {
		return string(b[:300])
	}
	return string(b)
}

// Read returns the MusicIDS.to_dict() representation of a file.
func (c *Client) Read(path string) (map[string]any, error) {
	data, err := c.run([]string{"read", "--path", path}, nil, 60*time.Second)
	if err != nil {
		return nil, err
	}
	var parsed map[string]any
	if err := parseJSON(data, &parsed); err != nil {
		return nil, err
	}
	if msg, ok := parsed["error"].(string); ok {
		return nil, fmt.Errorf("%s", msg)
	}
	return parsed, nil
}

// ReadBatch reads tags for many files with one interpreter start.
func (c *Client) ReadBatch(paths []string) (map[string]map[string]any, error) {
	payload, _ := json.Marshal(paths)
	data, err := c.run([]string{"read-batch"}, payload, 10*time.Minute)
	if err != nil {
		return nil, err
	}
	var parsed map[string]map[string]any
	if err := parseJSON(data, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

type WriteResult struct {
	FullPath string `json:"file_full_path"`
	OK       bool   `json:"ok"`
	Error    string `json:"error"`
}

// Write applies a list of update payloads (same shape as the old
// update_music_info input) and reports per-file results.
func (c *Client) Write(items []map[string]any) ([]WriteResult, error) {
	payload, _ := json.Marshal(items)
	data, err := c.run([]string{"write"}, payload, 30*time.Minute)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Results []WriteResult `json:"results"`
		Error   string        `json:"error"`
	}
	if err := parseJSON(data, &parsed); err != nil {
		return nil, err
	}
	if parsed.Error != "" {
		return nil, fmt.Errorf("%s", parsed.Error)
	}
	return parsed.Results, nil
}

// ScriptDir returns the directory containing tagcli.py.
func (c *Client) ScriptDir() string {
	return filepath.Dir(c.ScriptPath)
}
