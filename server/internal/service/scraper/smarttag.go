package scraper

import (
	"fmt"
	"sort"
	"sync"

	"github.com/xhongc/music-tag-web/server/internal/match"
)

// SmartTagClient ports smart_tag_resource.py: query four platforms in
// parallel, score every candidate against the file's existing tags and
// return the top matches.
type SmartTagClient struct {
	Registry *Registry
}

func (c *SmartTagClient) FetchLyric(songID string) (string, error) {
	return "", nil
}

func (c *SmartTagClient) FetchID3ByTitle(title any) ([]map[string]any, error) {
	info, ok := title.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("smart_tag 需要对象参数")
	}
	titleStr := str(info["title"])
	fullPath := str(info["full_path"])

	tags, err := c.Registry.Tags.Read(fullPath)
	if err != nil {
		return nil, err
	}
	artist := str(tags["artist"])
	album := str(tags["album"])

	var mu sync.Mutex
	var allSongs []map[string]any
	var wg sync.WaitGroup
	for _, resource := range []string{"qmusic", "netease", "migu", "kugou"} {
		wg.Add(1)
		go func(resource string) {
			defer wg.Done()
			client, err := c.Registry.Get(resource)
			if err != nil {
				return
			}
			songs, err := client.FetchID3ByTitle(titleStr)
			if err != nil {
				return
			}
			mu.Lock()
			for _, song := range songs {
				song["resource"] = resource
			}
			allSongs = append(allSongs, songs...)
			mu.Unlock()
		}(resource)
	}
	wg.Wait()

	type scored struct {
		song  map[string]any
		score int
	}
	var order []scored
	for _, song := range allSongs {
		name := str(song["name"])
		songArtist := str(song["artist"])
		songAlbum := str(song["album"])

		titleScore := match.Score(titleStr, name)
		artistScore := match.Artist(firstNonEmpty(artist, titleStr), songArtist)
		albumScore := match.Score(firstNonEmpty(album, titleStr), songAlbum)
		if artist != "" && artistScore == 0 {
			artistScore = -2
		}
		// 标题包含艺术家信息
		if artist == "" && artistScore >= 1 && titleScore >= 1 {
			titleScore = 2
		}
		total := titleScore + artistScore + albumScore
		song["score"] = total
		if titleScore == 0 {
			continue
		}
		order = append(order, scored{song: song, score: total})
	}
	sort.SliceStable(order, func(i, j int) bool {
		return order[i].score > order[j].score
	})
	result := []map[string]any{}
	for i, s := range order {
		if i >= 15 {
			break
		}
		result = append(result, s.song)
	}
	return result, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
