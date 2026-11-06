//go:build windows && mediae2e

package app

import (
	"encoding/json"
	"errors"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"screenpin/internal/process"
)

func TestRecorderWritesDecodableRegionMP4(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	encoder, err := findFFmpeg()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	complete := make(chan struct {
		path string
		err  error
	}, 1)
	frames := make(chan struct{}, 1)
	task, err := startRecorder(encoder, image.Rect(10, 10, 327, 249), directory, [][]string{{"-re", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000"}}, func(count int64) {
		if count >= 15 {
			select {
			case frames <- struct{}{}:
			default:
			}
		}
	}, func(path string, err error) {
		complete <- struct {
			path string
			err  error
		}{path, err}
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-frames:
	case result := <-complete:
		t.Fatalf("recorder stopped early: %v", result.err)
	case <-time.After(25 * time.Second):
		task.stop(false)
		t.Fatal("no video frames captured")
	}
	task.stop(true)
	var path string
	select {
	case result := <-complete:
		if result.err != nil {
			t.Fatal(result.err)
		}
		path = result.path
	case <-time.After(20 * time.Second):
		t.Fatal("recorder did not stop")
	}
	probe := exec.Command(filepath.Join(filepath.Dir(encoder), "ffprobe.exe"), "-v", "error", "-show_entries", "stream=codec_name,width,height,nb_frames", "-show_entries", "format=duration", "-of", "json", path)
	process.HideWindow(probe)
	output, err := probe.CombinedOutput()
	if err != nil {
		t.Fatalf("probe: %v %s", err, output)
	}
	var info struct {
		Streams []struct {
			Codec         string `json:"codec_name"`
			Width, Height int
			Frames        string `json:"nb_frames"`
		}
		Format struct{ Duration string }
	}
	if err := json.Unmarshal(output, &info); err != nil {
		t.Fatal(err)
	}
	if len(info.Streams) != 2 || info.Streams[1].Codec != "aac" || info.Streams[0].Codec != "h264" || info.Streams[0].Width != 318 || info.Streams[0].Height != 240 {
		t.Fatalf("wrong encoded region: %s", output)
	}
	count, _ := strconv.Atoi(info.Streams[0].Frames)
	duration, _ := strconv.ParseFloat(info.Format.Duration, 64)
	if count < 15 || duration <= 0 {
		t.Fatalf("empty media: %s", output)
	}
	decode := exec.Command(encoder, "-v", "error", "-i", path, "-f", "null", "-")
	process.HideWindow(decode)
	if output, err := decode.CombinedOutput(); err != nil {
		t.Fatalf("MP4 decode failed: %v %s", err, output)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("recording left partial files: %v %v", entries, err)
	}
	t.Logf("decoded H.264 %dx%d, %d frames, %.3f seconds", info.Streams[0].Width, info.Streams[0].Height, count, duration)
}

func TestCancelledRecordingLeavesNoVideo(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	encoder, err := findFFmpeg()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	complete := make(chan error, 1)
	task, err := startRecorder(encoder, image.Rect(10, 10, 170, 110), directory, nil, func(int64) {}, func(_ string, err error) { complete <- err })
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(600 * time.Millisecond)
	task.stop(false)
	select {
	case err := <-complete:
		if !errors.Is(err, errRecordingCancelled) {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("cancel did not finish")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("cancelled media remains: %v %v", entries, err)
	}
}

func TestEncoderGuardStopsItsProcess(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	encoder, err := findFFmpeg()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(encoder, "-hide_banner", "-loglevel", "error", "-re", "-f", "lavfi", "-i", "color=s=16x16:r=1", "-f", "null", "-")
	process.HideWindow(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	release, err := process.Guard(command)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	time.Sleep(200 * time.Millisecond)
	select {
	case err := <-done:
		release()
		t.Fatalf("encoder exited before guard was released: %v", err)
	default:
	}
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		t.Fatal("guard left the encoder running")
	}
}
