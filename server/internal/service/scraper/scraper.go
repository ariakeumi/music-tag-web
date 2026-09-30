// Package scraper ports applications/task/services/music_resource.py and the
// per-platform clients (netease/migu/qq/kugou/kuwo/acoustid/smart_tag).
package scraper

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/xhongc/music-tag-web/server/internal/tagclient"
)

var userAgents = []string{
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/60.0.3112.90 Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 9_1 like Mac OS X) AppleWebKit/601.1.46 (KHTML, like Gecko) Version/9.0 Mobile/13B143 Safari/601.1",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 9_1 like Mac OS X) AppleWebKit/601.1.46 (KHTML, like Gecko) Version/9.0 Mobile/13B143 Safari/601.1",
	"Mozilla/5.0 (Linux; Android 5.0; SM-G900P Build/LRX21T) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/59.0.3112.115 Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android 6.0; Nexus 5 Build/MRA58N) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/59.0.3112.90 Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android 5.1.1; Nexus 6 Build/LYZ28E) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/59.0.3112.90 Mobile Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 10_3_2 like Mac OS X) AppleWebKit/603.2.4 (KHTML, like Gecko) Mobile/14F89;GameHelper",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 10_0 like Mac OS X) AppleWebKit/602.1.38 (KHTML, like Gecko) Version/10.0 Mobile/14A300 Safari/602.1",
	"Mozilla/5.0 (iPad; CPU OS 10_0 like Mac OS X) AppleWebKit/602.1.38 (KHTML, like Gecko) Version/10.0 Mobile/14A300 Safari/602.1",
	"Mozilla/5.0 (Linux; U; Android 8.1.0; zh-cn; BKK-AL10 Build/HONORBKK-AL10) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/66.0.3359.126 MQQBrowser/10.6 Mobile Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.12; rv:46.0) Gecko/20100101 Firefox/46.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/59.0.3112.115 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/59.0.3112.90 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:46.0) Gecko/20100101 Firefox/46.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/51.0.2704.103 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/42.0.2311.135 Safari/537.36 Edge/13.10586",
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

func randomUA() string {
	return userAgents[rand.Intn(len(userAgents))]
}

// Client is the per-platform interface; title is a string for plain search
// platforms and a {"title","full_path"} map for smart_tag.
type Client interface {
	FetchLyric(songID string) (string, error)
	FetchID3ByTitle(title any) ([]map[string]any, error)
}

// Registry resolves resource names like the original MusicResource factory.
type Registry struct {
	Tags       *tagclient.Client
	FPCalcPath string
}

func (r *Registry) Get(resource string) (Client, error) {
	switch resource {
	case "netease":
		return &NetEaseClient{}, nil
	case "migu":
		return &MiGuClient{}, nil
	case "qmusic":
		return &QmusicClient{}, nil
	case "kugou":
		return &KugouClient{}, nil
	case "kuwo":
		return newKuwoClient(), nil
	case "acoustid":
		return &AcoustidClient{FPCalcPath: r.FPCalcPath}, nil
	case "smart_tag":
		return &SmartTagClient{Registry: r}, nil
	}
	return nil, fmt.Errorf("暂不支持该音乐平台")
}

// FetchLyric wraps a client lyric call with the original semantics: any
// error degrades to an empty string.
func (r *Registry) FetchLyric(resource, songID string) (string, error) {
	client, err := r.Get(resource)
	if err != nil {
		return "", err
	}
	lyric, err := client.FetchLyric(songID)
	if err != nil {
		return "", nil
	}
	return lyric, nil
}

// FetchID3ByTitle wraps a client search; fetch errors degrade to an empty
// list like MusicResource.fetch_id3_by_title did (unknown resources still
// raise, i.e. return an error).
func (r *Registry) FetchID3ByTitle(resource string, title any) ([]map[string]any, error) {
	client, err := r.Get(resource)
	if err != nil {
		return nil, err
	}
	songs, err := client.FetchID3ByTitle(title)
	if err != nil {
		return []map[string]any{}, nil
	}
	return songs, nil
}
