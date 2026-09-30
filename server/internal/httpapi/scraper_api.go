package httpapi

import (
	"net/http"
	"strings"
)

// HandleFetchID3ByTitle replicates POST /api/fetch_id3_by_title/.
func (s *Server) HandleFetchID3ByTitle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Resource string `json:"resource"`
		Title    string `json:"title"`
		FullPath string `json:"full_path"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		Failure(w, "参数错误")
		return
	}
	var query any
	switch body.Resource {
	case "acoustid":
		query = body.FullPath
	case "smart_tag":
		query = map[string]any{"title": body.Title, "full_path": body.FullPath}
	default:
		query = body.Title
	}
	songs, err := s.Scraper.FetchID3ByTitle(body.Resource, query)
	if err != nil {
		Failure(w, err.Error())
		return
	}
	if songs == nil {
		songs = []map[string]any{}
	}
	Success(w, songs)
}

// HandleFetchLyric replicates POST /api/fetch_lyric/.
func (s *Server) HandleFetchLyric(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Resource string `json:"resource"`
		SongID   string `json:"song_id"`
	}
	if err := DecodeJSON(r, &body); err != nil {
		Failure(w, "参数错误")
		return
	}
	lyric, err := s.Scraper.FetchLyric(body.Resource, body.SongID)
	if err != nil {
		Success(w, "未找到歌词 "+err.Error())
		return
	}
	Success(w, lyric)
}

// HandleTranslationLyc replicates POST /api/translation_lyc/: translate the
// text-only part of each lyric line and inline it below the original.
func (s *Server) HandleTranslationLyc(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Lyc string `json:"lyc"`
	}
	if err := DecodeJSON(r, &body); err != nil || body.Lyc == "" {
		Failure(w, "参数错误")
		return
	}
	var rawList, cleanList []string
	for _, line := range strings.Split(body.Lyc, "\n") {
		if line == "" {
			continue
		}
		cleanLine := line
		if idx := strings.LastIndex(line, "]"); idx >= 0 {
			cleanLine = line[idx+1:]
		}
		cleanLine = strings.TrimSpace(cleanLine)
		if cleanLine == "" {
			continue
		}
		rawList = append(rawList, line)
		cleanList = append(cleanList, cleanLine)
	}
	results, err := s.Translate.TranslateLyc(strings.Join(cleanList, "\n"))
	if err != nil {
		Failure(w, err.Error())
		return
	}
	var newLyc []string
	for index, result := range strings.Split(results, "\n") {
		if index >= len(rawList) {
			break
		}
		rawSrc := rawList[index]
		src := cleanList[index]
		if !resultEmpty(result) {
			if stripSpaces(src) == stripSpaces(result) {
				newLyc = append(newLyc, rawSrc)
			} else {
				newLyc = append(newLyc, rawSrc+"\n「"+result+"」\n")
			}
		} else {
			newLyc = append(newLyc, rawSrc)
		}
	}
	Success(w, strings.Join(newLyc, "\n"))
}

func resultEmpty(s string) bool { return s == "" }

func stripSpaces(s string) string {
	return strings.ReplaceAll(s, " ", "")
}
