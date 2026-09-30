package scraper

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AcoustidClient ports services/acoust.py + component/mz (pyacoustid):
// fingerprint via the bundled fpcalc binary, then a lookup call.
type AcoustidClient struct {
	FPCalcPath string
}

const acoustidAPIKey = "cSpUJKpD"
const acoustidAPIBase = "http://api.acoustid.org/v2/"
const acoustidMaxLength = 120

// the pyacoustid client rate-limits lookups to 3/second
var acoustidMu sync.Mutex
var acoustidLastCall time.Time

func (c *AcoustidClient) FetchLyric(songID string) (string, error) {
	return "", fmt.Errorf("暂不支持该音乐平台")
}

func (c *AcoustidClient) FetchID3ByTitle(title any) ([]map[string]any, error) {
	path, _ := title.(string)
	duration, fingerprint, err := c.fingerprint(path)
	if err != nil {
		return nil, err
	}
	results, err := acoustidLookup(duration, fingerprint)
	if err != nil {
		return nil, err
	}
	songs := []map[string]any{}
	for _, r := range results {
		songs = append(songs, map[string]any{
			"id":        r.id,
			"name":      r.title,
			"artist":    r.artist,
			"artist_id": "",
			"album":     r.album,
			"album_id":  "",
			"album_img": "",
			"year":      "",
		})
	}
	return songs, nil
}

func (c *AcoustidClient) fingerprint(path string) (int, string, error) {
	command := exec.Command(c.FPCalcPath, "-length", strconv.Itoa(acoustidMaxLength), path)
	output, err := command.Output()
	if err != nil {
		return 0, "", fmt.Errorf("fpcalc invocation failed: %w", err)
	}
	duration, fp := 0, ""
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "DURATION":
			duration, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
		case "FINGERPRINT":
			fp = strings.TrimSpace(parts[1])
		}
	}
	if fp == "" {
		return 0, "", fmt.Errorf("fpcalc returned no fingerprint for %s", path)
	}
	return duration, fp, nil
}

type acoustidMatch struct {
	score  float64
	id     string
	title  string
	artist string
	album  string
}

func acoustidLookup(duration int, fingerprint string) ([]acoustidMatch, error) {
	acoustidMu.Lock()
	since := time.Since(acoustidLastCall)
	if since < 333*time.Millisecond {
		time.Sleep(333*time.Millisecond - since)
	}
	acoustidLastCall = time.Now()
	acoustidMu.Unlock()

	form := url.Values{}
	form.Set("format", "json")
	form.Set("client", acoustidAPIKey)
	form.Set("duration", strconv.Itoa(duration))
	form.Set("fingerprint", fingerprint)
	form.Set("meta", "recordings releasegroups")
	req, err := http.NewRequest(http.MethodPost, acoustidAPIBase+"lookup",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Status  string `json:"status"`
		Results []struct {
			Score      float64 `json:"score"`
			Recordings []struct {
				ID      string `json:"id"`
				Title   string `json:"title"`
				Artists []struct {
					Name       string `json:"name"`
					JoinPhrase string `json:"joinphrase"`
				} `json:"artists"`
				Releasegroups []struct {
					Title string `json:"title"`
				} `json:"releasegroups"`
			} `json:"recordings"`
		} `json:"results"`
	}
	if err := jsonUnmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if parsed.Status != "ok" {
		return nil, fmt.Errorf("status: %s", parsed.Status)
	}
	matches := []acoustidMatch{}
	for _, result := range parsed.Results {
		for _, recording := range result.Recordings {
			artist := ""
			for _, a := range recording.Artists {
				artist += a.Name + a.JoinPhrase
			}
			album := ""
			if len(recording.Releasegroups) > 0 {
				album = recording.Releasegroups[0].Title
			}
			matches = append(matches, acoustidMatch{
				score:  result.Score,
				id:     recording.ID,
				title:  recording.Title,
				artist: artist,
				album:  album,
			})
		}
	}
	return matches, nil
}
