// Package youtube connects an organisation's YouTube channel and reads
// its videos' analytics, so CAVI can see which clips held viewers and won
// subscribers, and plan more like them.
//
// It talks to Google's REST endpoints directly: OAuth 2.0 for consent
// (read-only scopes), the YouTube Data API for the channel's uploads and
// the YouTube Analytics API for their numbers. Refresh tokens are
// encrypted at rest; access tokens are never stored.
package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Scopes are read-only: the channel's videos and their analytics.
var Scopes = []string{
	"https://www.googleapis.com/auth/youtube.readonly",
	"https://www.googleapis.com/auth/yt-analytics.readonly",
}

// ErrRevoked means the channel's consent was withdrawn or expired:
// connecting again is the only fix.
var ErrRevoked = errors.New("youtube: access was revoked or expired")

// Client calls Google. The URLs are fields so tests can point them at a
// fake server.
type Client struct {
	ClientID, ClientSecret string
	// RedirectURL is where Google sends people back after consent.
	RedirectURL string
	HTTP        *http.Client

	AuthURL, TokenURL, DataURL, AnalyticsURL string
}

// NewClient returns a Client for Google's real endpoints.
func NewClient(clientID, clientSecret, redirectURL string) *Client {
	return &Client{
		ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
		HTTP:         &http.Client{Timeout: 30 * time.Second},
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		DataURL:      "https://www.googleapis.com/youtube/v3",
		AnalyticsURL: "https://youtubeanalytics.googleapis.com/v2",
	}
}

// ConsentURL is where to send someone to connect their channel. state
// ties the reply to their request; offline access and a forced consent
// screen make sure Google returns a refresh token.
func (c *Client) ConsentURL(state string) string {
	q := url.Values{
		"client_id":              {c.ClientID},
		"redirect_uri":           {c.RedirectURL},
		"response_type":          {"code"},
		"scope":                  {strings.Join(Scopes, " ")},
		"access_type":            {"offline"},
		"prompt":                 {"consent"},
		"include_granted_scopes": {"true"},
		"state":                  {state},
	}
	return c.AuthURL + "?" + q.Encode()
}

type tokenReply struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
}

func (c *Client) token(ctx context.Context, form url.Values) (tokenReply, error) {
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenReply{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return tokenReply{}, fmt.Errorf("youtube: token request: %w", err)
	}
	defer resp.Body.Close()
	var t tokenReply
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&t)
	if t.Error == "invalid_grant" {
		return tokenReply{}, ErrRevoked
	}
	if resp.StatusCode != http.StatusOK || t.AccessToken == "" {
		return tokenReply{}, fmt.Errorf("youtube: token request failed (%d %s)", resp.StatusCode, t.Error)
	}
	return t, nil
}

// Exchange turns the code Google returned into tokens: the access token
// and the refresh token to keep.
func (c *Client) Exchange(ctx context.Context, code string) (access, refresh string, scopes []string, err error) {
	t, err := c.token(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {c.RedirectURL}})
	if err != nil {
		return "", "", nil, err
	}
	if t.RefreshToken == "" {
		return "", "", nil, errors.New("youtube: Google returned no refresh token")
	}
	return t.AccessToken, t.RefreshToken, strings.Fields(t.Scope), nil
}

// Access gets a fresh access token from a refresh token.
func (c *Client) Access(ctx context.Context, refresh string) (string, error) {
	t, err := c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
	return t.AccessToken, err
}

// get calls a Google API with an access token and decodes JSON into v.
func (c *Client) get(ctx context.Context, access, rawURL string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("youtube: request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrRevoked
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		return &APIError{Status: resp.StatusCode, Message: e.Error.Message}
	}
	return json.Unmarshal(body, v)
}

// APIError is an error reply from a Google API.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("youtube: API error %d: %s", e.Status, e.Message) }

// Channel is the connected channel.
type Channel struct {
	ID, Title string
	// Uploads is the playlist of the channel's uploads.
	Uploads string
}

// MyChannel returns the channel the access token belongs to.
func (c *Client) MyChannel(ctx context.Context, access string) (Channel, error) {
	var r struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
			ContentDetails struct {
				RelatedPlaylists struct {
					Uploads string `json:"uploads"`
				} `json:"relatedPlaylists"`
			} `json:"contentDetails"`
		} `json:"items"`
	}
	if err := c.get(ctx, access, c.DataURL+"/channels?part=snippet,contentDetails&mine=true", &r); err != nil {
		return Channel{}, err
	}
	if len(r.Items) == 0 {
		return Channel{}, errors.New("youtube: this Google account has no YouTube channel")
	}
	it := r.Items[0]
	return Channel{ID: it.ID, Title: it.Snippet.Title, Uploads: it.ContentDetails.RelatedPlaylists.Uploads}, nil
}

// Video is one upload, with its live view and like counts (the
// Analytics API's numbers lag by a day or two; these don't).
type Video struct {
	ID          string
	Title       string
	PublishedAt time.Time
	DurationMS  int64
	Views       int64
	Likes       int64
}

// MaxVideos is how many of the newest uploads are read.
const MaxVideos = 200

// Uploads lists the channel's newest uploads (up to MaxVideos) with
// their lengths.
func (c *Client) Uploads(ctx context.Context, access string, ch Channel) ([]Video, error) {
	var ids []string
	page := ""
	for len(ids) < MaxVideos {
		var r struct {
			NextPageToken string `json:"nextPageToken"`
			Items         []struct {
				ContentDetails struct {
					VideoID string `json:"videoId"`
				} `json:"contentDetails"`
			} `json:"items"`
		}
		u := c.DataURL + "/playlistItems?part=contentDetails&maxResults=50&playlistId=" + url.QueryEscape(ch.Uploads)
		if page != "" {
			u += "&pageToken=" + url.QueryEscape(page)
		}
		if err := c.get(ctx, access, u, &r); err != nil {
			return nil, err
		}
		for _, it := range r.Items {
			ids = append(ids, it.ContentDetails.VideoID)
		}
		if page = r.NextPageToken; page == "" {
			break
		}
	}
	if len(ids) > MaxVideos {
		ids = ids[:MaxVideos]
	}
	var out []Video
	for from := 0; from < len(ids); from += 50 {
		batch := ids[from:min(from+50, len(ids))]
		var r struct {
			Items []struct {
				ID      string `json:"id"`
				Snippet struct {
					Title       string    `json:"title"`
					PublishedAt time.Time `json:"publishedAt"`
				} `json:"snippet"`
				ContentDetails struct {
					Duration string `json:"duration"`
				} `json:"contentDetails"`
				Statistics struct {
					ViewCount string `json:"viewCount"`
					LikeCount string `json:"likeCount"`
				} `json:"statistics"`
			} `json:"items"`
		}
		if err := c.get(ctx, access, c.DataURL+"/videos?part=snippet,contentDetails,statistics&id="+url.QueryEscape(strings.Join(batch, ",")), &r); err != nil {
			return nil, err
		}
		for _, it := range r.Items {
			views, _ := strconv.ParseInt(it.Statistics.ViewCount, 10, 64)
			likes, _ := strconv.ParseInt(it.Statistics.LikeCount, 10, 64)
			out = append(out, Video{ID: it.ID, Title: it.Snippet.Title, PublishedAt: it.Snippet.PublishedAt,
				DurationMS: isoDuration(it.ContentDetails.Duration), Views: views, Likes: likes})
		}
	}
	return out, nil
}

var isoPattern = regexp.MustCompile(`^P(?:(\d+)D)?T?(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?$`)

// isoDuration parses YouTube's ISO 8601 durations ("PT1M13S"); 0 if it
// can't.
func isoDuration(s string) int64 {
	m := isoPattern.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	n := func(v string) float64 { f, _ := strconv.ParseFloat(v, 64); return f }
	secs := n(m[1])*86400 + n(m[2])*3600 + n(m[3])*60 + n(m[4])
	return int64(secs * 1000)
}

// Metrics are a video's lifetime numbers.
type Metrics struct {
	Views             int64
	WatchMinutes      float64
	AvgViewSeconds    float64
	AvgViewPercentage float64
	SubscribersGained int64
	Likes             int64
	Shares            int64
}

// Analytics returns each video's numbers since the channel started (up to
// MaxVideos videos, most viewed first). YouTube's figures lag by a day or
// two.
func (c *Client) Analytics(ctx context.Context, access string, today time.Time) (map[string]Metrics, error) {
	q := url.Values{
		"ids":        {"channel==MINE"},
		"startDate":  {"2005-04-23"},
		"endDate":    {today.Format("2006-01-02")},
		"dimensions": {"video"},
		"metrics":    {"views,estimatedMinutesWatched,averageViewDuration,averageViewPercentage,subscribersGained,likes,shares"},
		"sort":       {"-views"},
		"maxResults": {strconv.Itoa(MaxVideos)},
	}
	var r struct {
		ColumnHeaders []struct {
			Name string `json:"name"`
		} `json:"columnHeaders"`
		Rows [][]any `json:"rows"`
	}
	if err := c.get(ctx, access, c.AnalyticsURL+"/reports?"+q.Encode(), &r); err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range r.ColumnHeaders {
		col[h.Name] = i
	}
	num := func(row []any, name string) float64 {
		i, ok := col[name]
		if !ok || i >= len(row) {
			return 0
		}
		f, _ := row[i].(float64)
		return f
	}
	out := map[string]Metrics{}
	for _, row := range r.Rows {
		i, ok := col["video"]
		if !ok || i >= len(row) {
			continue
		}
		id, _ := row[i].(string)
		if id == "" {
			continue
		}
		out[id] = Metrics{
			Views: int64(num(row, "views")), WatchMinutes: num(row, "estimatedMinutesWatched"),
			AvgViewSeconds: num(row, "averageViewDuration"), AvgViewPercentage: num(row, "averageViewPercentage"),
			SubscribersGained: int64(num(row, "subscribersGained")), Likes: int64(num(row, "likes")), Shares: int64(num(row, "shares")),
		}
	}
	return out, nil
}
