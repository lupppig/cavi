package framing

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// Default locations of the face finder in the media worker image.
const (
	DefaultHelper = "cavi-faces"
	DefaultModel  = "/usr/local/share/cavi/yunet.onnx"
)

// maxFaces matches the helper's own limit.
const maxFaces = 50

// Helper is a running cavi-faces process: frames go in on its stdin and
// one line of faces comes back for each.
type Helper struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
	stderr bytes.Buffer
}

var _ Detector = (*Helper)(nil)

// StartHelper starts bin (cavi-faces) with model for frames of w×h. Stop
// it with Close; ctx ending kills it.
func StartHelper(ctx context.Context, bin, model string, w, h int) (*Helper, error) {
	hp := &Helper{cmd: exec.CommandContext(ctx, bin, model, strconv.Itoa(w), strconv.Itoa(h))}
	hp.cmd.Stderr = &hp.stderr
	var err error
	if hp.in, err = hp.cmd.StdinPipe(); err != nil {
		return nil, err
	}
	out, err := hp.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	hp.out = bufio.NewReader(out)
	if err := hp.cmd.Start(); err != nil {
		return nil, fmt.Errorf("framing: start face finder: %w", err)
	}
	return hp, nil
}

// Detect implements Detector.
func (hp *Helper) Detect(bgr []byte) ([]Box, error) {
	if _, err := hp.in.Write(bgr); err != nil {
		return nil, hp.failed(err)
	}
	line, err := hp.out.ReadString('\n')
	if err != nil {
		return nil, hp.failed(err)
	}
	return parseFaces(line)
}

func (hp *Helper) failed(err error) error {
	msg := strings.TrimSpace(hp.stderr.String())
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return fmt.Errorf("framing: face finder stopped: %v %s", err, msg)
}

// Close stops the helper.
func (hp *Helper) Close() error {
	_ = hp.in.Close()
	return hp.cmd.Wait()
}

// parseFaces reads one helper line: "n x y w h score ...".
func parseFaces(line string) ([]Box, error) {
	f := strings.Fields(line)
	if len(f) == 0 {
		return nil, fmt.Errorf("framing: empty face finder output")
	}
	n, err := strconv.Atoi(f[0])
	if err != nil || n < 0 || n > maxFaces || len(f) != 1+5*n {
		return nil, fmt.Errorf("framing: malformed face finder output")
	}
	out := make([]Box, n)
	for i := range out {
		var v [5]float64
		for k := range v {
			if v[k], err = strconv.ParseFloat(f[1+5*i+k], 64); err != nil {
				return nil, fmt.Errorf("framing: malformed face finder output")
			}
		}
		out[i] = Box{X: v[0], Y: v[1], W: v[2], H: v[3], Score: v[4]}
	}
	return out, nil
}
