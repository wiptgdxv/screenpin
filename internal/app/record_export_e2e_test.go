//go:build windows && mediae2e

package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRecorderSegmentJoinAndTrimmedExports(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	encoder, err := findFFmpeg()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var parts []string
	for _, pattern := range []string{"testsrc2", "smptebars"} {
		path := filepath.Join(dir, "录屏_"+pattern+".mp4")
		err = runMediaExport(encoder, path, []string{"-f", "lavfi", "-i", pattern + "=size=160x96:rate=30", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "1", "-c:v", "libx264", "-c:a", "aac"})
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, path)
	}
	source, err := joinRecordingSegments(encoder, parts)
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"mp4", "gif", "webp"} {
		path := filepath.Join(dir, "trim."+format)
		if err := exportRecording(source, path, videoEdit{format: format, start: .6, end: 1.4}); err != nil {
			t.Fatal(err)
		}
		if format == "webp" {
			data, err := os.ReadFile(path)
			if err != nil || len(data) < 12 || string(data[8:12]) != "WEBP" || bytes.Count(data, []byte("ANMF")) < 2 {
				t.Fatalf("expected animated WebP: %v", err)
			}
			continue
		}
		probe := exec.Command(filepath.Join(filepath.Dir(encoder), "ffprobe.exe"), "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path)
		data, err := probe.CombinedOutput()
		duration, parseErr := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err != nil || parseErr != nil || duration < .68 || duration > .95 {
			t.Fatalf("%s duration: %s (%v)", format, data, err)
		}
		decode := exec.Command(encoder, "-v", "error", "-i", path, "-f", "null", "-")
		if data, err := decode.CombinedOutput(); err != nil {
			t.Fatalf("%s decode: %v %s", format, err, data)
		}
	}
}
