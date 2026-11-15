package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// YTDLP downloads videos with yt-dlp, limited to the supported platforms'
// extractors. It needs yt-dlp and a JavaScript runtime (Node) for YouTube.
type YTDLP struct {
	Bin string
	// Timeout bounds each download (default an hour).
	Timeout time.Duration
}

var _ Fetcher = YTDLP{}

// extractors are the yt-dlp extractors allowed: single videos and clips.
const extractors = "youtube,YoutubeYtBe,twitch:vod,twitch:clips,kick:vod,kick:clip"

type videoInfo struct {
	Type           string  `json:"_type"`
	Title          string  `json:"title"`
	Duration       float64 `json:"duration"`
	LiveStatus     string  `json:"live_status"`
	IsLive         bool    `json:"is_live"`
	FilesizeApprox int64   `json:"filesize_approx"`
}

// Fetch implements Fetcher: it reads the video's details first (refusing
// playlists, live streams and over-long videos), then downloads it, at most
// 1080p, preferring H.264 and AAC in MP4.
func (y YTDLP) Fetch(ctx context.Context, link, dir string) (Fetched, error) {
	timeout := y.Timeout
	if timeout <= 0 {
		timeout = importTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	base := []string{"--ignore-config", "--no-cache-dir", "--no-playlist", "--js-runtimes", "node", "--use-extractors", extractors}

	out, err := y.run(ctx, dir, append(base, "--dump-single-json", "--", link)...)
	if err != nil {
		return Fetched{}, err
	}
	var info videoInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return Fetched{}, fmt.Errorf("yt-dlp: unreadable video details: %w", err)
	}
	switch {
	case info.Type != "" && info.Type != "video":
		return Fetched{}, &ImportError{Reason: "That link is a playlist or channel. Paste a link to one video."}
	case info.IsLive || info.LiveStatus == "is_live" || info.LiveStatus == "is_upcoming" || info.LiveStatus == "post_live":
		return Fetched{}, &ImportError{Reason: "That’s a live stream. Import it once it has finished and the recording is up."}
	case time.Duration(info.Duration*float64(time.Second)) > MaxImportDuration:
		return Fetched{}, &ImportError{Reason: "Videos longer than 4 hours can’t be imported."}
	case info.FilesizeApprox > MaxUploadBytes:
		return Fetched{}, &ImportError{Reason: msgImportTooBig}
	}
	infoPath := filepath.Join(dir, "info.json")
	if err := os.WriteFile(infoPath, out, 0o600); err != nil {
		return Fetched{}, err
	}

	out, err = y.run(ctx, dir, "--ignore-config", "--no-cache-dir", "--js-runtimes", "node",
		"--load-info-json", infoPath,
		"-f", "bv*+ba/b", "-S", "res:1080,vcodec:h264,acodec:aac", "--merge-output-format", "mp4",
		"--max-filesize", "5G", "--no-progress", "--restrict-filenames",
		"-o", filepath.Join(dir, "source.%(ext)s"), "--print", "after_move:filepath")
	if err != nil {
		return Fetched{}, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	path := strings.TrimSpace(lines[len(lines)-1])
	if path == "" {
		// yt-dlp skips files over --max-filesize without failing.
		return Fetched{}, &ImportError{Reason: msgImportTooBig}
	}
	return Fetched{Path: path, Title: info.Title}, nil
}

// refusals are yt-dlp errors that mean the video can't be had, in words
// for the user. Anything else is retried.
var refusals = []struct{ signal, reason string }{
	{"Unsupported URL", "That link isn’t a video CAVI can import. Paste a link to one YouTube, Twitch or Kick video."},
	{"not a bot", "YouTube is refusing downloads from our server right now. Try again later, or download the video and upload it."},
	{"Private video", "That video is private, so it can’t be imported."},
	{"members-only", "That video is for channel members only, so it can’t be imported."},
	{"Join this channel", "That video is for channel members only, so it can’t be imported."},
	{"confirm your age", "That video is age-restricted, so it can’t be imported."},
	{"Video unavailable", "That video is unavailable: it may have been removed or blocked."},
	{"This video is unavailable", "That video is unavailable: it may have been removed or blocked."},
	{"has been removed", "That video has been removed."},
	{"HTTP Error 404", "That video couldn’t be found. Check the link."},
	{"Requested format is not available", "That video has no downloadable format we can use."},
}

func (y YTDLP) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	bin := y.Bin
	if bin == "" {
		bin = "yt-dlp"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	// Keep yt-dlp's own files (cache, config lookups) in the work directory.
	cmd.Env = append(os.Environ(), "HOME="+dir, "XDG_CACHE_HOME="+dir, "XDG_CONFIG_HOME="+dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("yt-dlp: timed out or was cancelled")
	}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return nil, &ImportError{Reason: "Importing from links isn’t available on this server yet."}
		}
		msg := stderr.String()
		for _, r := range refusals {
			if strings.Contains(msg, r.signal) {
				return nil, &ImportError{Reason: r.reason}
			}
		}
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return nil, fmt.Errorf("yt-dlp failed: %s", strings.TrimSpace(msg))
	}
	return stdout.Bytes(), nil
}
