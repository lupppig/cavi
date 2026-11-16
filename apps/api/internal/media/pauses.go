package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/uptrace/bun"
)

// Pause is a stretch with nobody speaking, in the file's milliseconds.
type Pause struct {
	StartMS, EndMS int64
}

// MarshalJSON stores a pause compactly, as [start, end].
func (p Pause) MarshalJSON() ([]byte, error) { return json.Marshal([2]int64{p.StartMS, p.EndMS}) }

// UnmarshalJSON reads [start, end].
func (p *Pause) UnmarshalJSON(b []byte) error {
	var v [2]int64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	p.StartMS, p.EndMS = v[0], v[1]
	return nil
}

// Pause detection.
const (
	// pauseFPS is how often loudness is measured.
	pauseFPS = 25
	// MinPauseMS is the shortest pause recorded.
	MinPauseMS = 300
	// levelWindow is how many seconds of sound around a moment set the
	// speech level it is compared with: voices, rooms and microphones
	// change through a video.
	levelWindow = 20
	// pauseBelow is how far below the speech level (dB) counts as a
	// pause: syllables dip 6 to 12 dB, gaps in speech far more.
	pauseBelow = 15.0
	// silentDB is quiet whatever the background.
	silentDB = -55.0
)

// FindPauses finds the pauses in loudness levels (RMS, 0 to 1) measured
// fps times a second: stretches at least MinPauseMS long that are
// pauseBelow under the speech level around them (the 90th percentile
// over levelWindow seconds) or silent. Speech over sound that never drops
// much (a music bed) has few pauses, which is the safe failure: nothing
// gets cut.
func FindPauses(levels []float64, fps int) []Pause {
	if len(levels) == 0 || fps <= 0 {
		return []Pause{}
	}
	db := make([]float64, len(levels))
	for i, v := range levels {
		db[i] = 20 * math.Log10(max(v, 1e-6))
	}
	// The speech level per second, from the window around it.
	secs := (len(db) + fps - 1) / fps
	level := make([]float64, secs)
	for s := range secs {
		from := max(0, (s-levelWindow/2)*fps)
		to := min(len(db), (s+levelWindow/2)*fps)
		w := append([]float64(nil), db[from:to]...)
		sort.Float64s(w)
		level[s] = w[len(w)*9/10]
	}
	quiet := func(i int) bool { return db[i] < silentDB || db[i] < level[i/fps]-pauseBelow }
	out := []Pause{}
	minFrames := MinPauseMS * fps / 1000
	for i := 0; i < len(db); {
		if !quiet(i) {
			i++
			continue
		}
		j := i
		for j < len(db) && quiet(j) {
			j++
		}
		if j-i >= minFrames {
			out = append(out, Pause{StartMS: int64(i) * 1000 / int64(fps), EndMS: int64(j) * 1000 / int64(fps)})
		}
		i = j
	}
	return out
}

// Pauses returns a file's pauses, if they have been found. The caller has
// already authorised access to the file.
func Pauses(ctx context.Context, db bun.IDB, orgID, assetID string) ([]Pause, bool, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT pauses FROM media_assets WHERE id = ? AND organisation_id = ? AND pauses IS NOT NULL`,
		assetID, orgID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load pauses: %w", err)
	}
	var ps []Pause
	if err := json.Unmarshal(raw, &ps); err != nil {
		return nil, false, fmt.Errorf("pauses for %s: %w", assetID, err)
	}
	return ps, true, nil
}
