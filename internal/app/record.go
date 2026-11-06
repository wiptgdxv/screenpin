package app

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"screenpin/internal/process"

	"github.com/egoist/mygo"
)

var errRecordingCancelled = errors.New("已取消录屏")

type recordTask struct {
	command  *exec.Cmd
	input    io.WriteCloser
	done     chan struct{}
	once     sync.Once
	discard  atomic.Bool
	stopping atomic.Bool
	frames   atomic.Int64
}

func (task *recordTask) stop(keep bool) {
	if !keep {
		task.discard.Store(true)
	}
	task.once.Do(func() {
		task.stopping.Store(true)
		go func() {
			_, _ = io.WriteString(task.input, "q\n")
			select {
			case <-task.done:
			case <-time.After(30 * time.Second):
				_ = task.command.Process.Kill()
			}
		}()
	})
}

type logTail struct {
	mu   sync.Mutex
	data []byte
}

func (t *logTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data = append(t.data, p...)
	if len(t.data) > 8192 {
		t.data = bytes.Clone(t.data[len(t.data)-8192:])
	}
	return len(p), nil
}
func (t *logTail) String() string { t.mu.Lock(); defer t.mu.Unlock(); return string(t.data) }

func findFFmpeg() (string, error) {
	executable, _ := os.Executable()
	for _, directory := range []string{filepath.Dir(executable), filepath.Dir(filepath.Dir(executable)), "."} {
		path := filepath.Join(directory, "tools", "ffmpeg", "ffmpeg.exe")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return filepath.Abs(path)
		}
	}
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("未找到录屏编码器，请将 FFmpeg 放入 tools/ffmpeg")
}

func videoDirectory() string {
	if directory := os.Getenv("SCREENPIN_VIDEO_DIR"); filepath.IsAbs(directory) {
		return directory
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "Screenpin", "Videos")
	}
	return filepath.Join(home, "Videos", "Screenpin")
}

func latestRecording(directory string) string {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return ""
	}
	// Finalized recordings have a sortable timestamp in their filename.
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		name := entry.Name()
		if !strings.HasPrefix(name, "录屏_") || !strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".partial.mp4") {
			continue
		}
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			return filepath.Join(directory, name)
		}
	}
	return ""
}

func (a *app) recordingSaveDirectory() string {
	if a.videoSaveDirectory != "" {
		return a.videoSaveDirectory
	}
	return videoDirectory()
}

func (a *app) chooseVideoSaveDirectory() {
	if a.window == nil {
		return
	}
	directory := a.recordingSaveDirectory()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.window, Title: "选择录屏保存目录", DefaultPath: directory,
			Directory: true, CreateDirectories: true,
		})
		mygo.RunOnMain(func() {
			if err != nil {
				a.status = "选择目录失败：" + err.Error()
			} else if len(paths) > 0 && filepath.IsAbs(paths[0]) {
				a.videoSaveDirectory = filepath.Clean(paths[0])
				a.status = "已更新录屏保存目录"
				a.persistAppearance()
			}
			a.window.Invalidate()
		})
	}()
}

func (a *app) openVideoDirectory() {
	directory := a.recordingSaveDirectory()
	if err := os.MkdirAll(directory, 0700); err != nil {
		a.status = err.Error()
		return
	}
	if err := mygo.Shell.OpenPath(directory); err != nil {
		a.status = "打开目录失败：" + err.Error()
	}
}

func (a *app) openRecording(path string) {
	if err := mygo.Shell.OpenPath(path); err != nil {
		a.status = "打开视频失败：" + err.Error()
		a.showPanel()
		a.window.Invalidate()
	}
}

func recordingArgs(bounds image.Rectangle, path string, audioInputs [][]string) []string {
	args := []string{"-hide_banner", "-loglevel", "warning", "-y", "-thread_queue_size", "256", "-f", "gdigrab", "-draw_mouse", "1", "-framerate", "30", "-offset_x", strconv.Itoa(bounds.Min.X), "-offset_y", strconv.Itoa(bounds.Min.Y), "-video_size", fmt.Sprintf("%dx%d", bounds.Dx(), bounds.Dy()), "-i", "desktop"}
	if len(audioInputs) > 0 {
		for _, input := range audioInputs {
			args = append(args, input...)
		}
		args = append(args, "-map", "0:v:0")
		if len(audioInputs) == 2 {
			args = append(args, "-filter_complex", "[1:a][2:a]amix=inputs=2:normalize=1[mix]", "-map", "[mix]")
		} else {
			args = append(args, "-map", "1:a:0")
		}
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	} else {
		args = append(args, "-an")
	}
	return append(args, "-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2", "-c:v", "libx264", "-preset", "veryfast", "-crf", "16", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-progress", "pipe:1", "-stats_period", "0.5", path)
}

func startRecorder(executable string, bounds image.Rectangle, directory string, audioInputs [][]string, progress func(int64), complete func(string, error)) (*recordTask, error) {
	if bounds.Dx() < 2 || bounds.Dy() < 2 {
		return nil, fmt.Errorf("录制区域过小")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	placeholder, err := os.CreateTemp(directory, "screenpin-*.partial.mp4")
	if err != nil {
		return nil, err
	}
	temporary := placeholder.Name()
	_ = placeholder.Close()
	command := exec.Command(executable, recordingArgs(bounds, temporary, audioInputs)...)
	process.HideWindow(command)
	output, err := command.StdoutPipe()
	if err != nil {
		_ = os.Remove(temporary)
		return nil, err
	}
	input, err := command.StdinPipe()
	if err != nil {
		_ = output.Close()
		_ = os.Remove(temporary)
		return nil, err
	}
	tail := &logTail{}
	command.Stderr = tail
	task := &recordTask{command: command, input: input, done: make(chan struct{})}
	if err := command.Start(); err != nil {
		_ = input.Close()
		_ = output.Close()
		_ = os.Remove(temporary)
		return nil, err
	}
	release, err := process.Guard(command)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		_ = input.Close()
		_ = output.Close()
		_ = os.Remove(temporary)
		return nil, fmt.Errorf("无法管理录屏进程：%w", err)
	}
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			key, value, ok := strings.Cut(scanner.Text(), "=")
			if ok && key == "frame" {
				frames, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
				task.frames.Store(frames)
				progress(frames)
			}
		}
	}()
	go func() {
		defer release()
		<-scanDone
		err := command.Wait()
		_ = input.Close()
		close(task.done)
		if task.discard.Load() {
			_ = os.Remove(temporary)
			complete("", errRecordingCancelled)
			return
		}
		if err != nil || task.frames.Load() < 1 {
			reason := strings.TrimSpace(tail.String())
			_ = os.Remove(temporary)
			complete("", fmt.Errorf("录屏未完成：%v %s", err, reason))
			return
		}
		path := filepath.Join(directory, "录屏_"+time.Now().Format("20060102_150405.000")+"_"+strings.TrimSuffix(strings.TrimPrefix(filepath.Base(temporary), "screenpin-"), ".partial.mp4")+".mp4")
		if err := os.Rename(temporary, path); err != nil {
			complete("", fmt.Errorf("保存失败，临时文件保留在 %s：%w", temporary, err))
			return
		}
		complete(path, nil)
	}()
	return task, nil
}

func (a *app) recordVideo() {
	if a.recordTask != nil {
		a.stopRecording(true)
		return
	}
	if a.recordPreview != nil {
		a.recordPreview.present()
		return
	}
	if !mygo.WebViewAvailable() {
		a.status = "录屏预览需要 Microsoft Edge WebView2 Runtime，请安装后重试"
		a.showPanel()
		a.window.Invalidate()
		return
	}
	encoder, err := findFFmpeg()
	if err != nil {
		a.status = err.Error()
		a.window.Show()
		a.window.Invalidate()
		return
	}
	a.beginSelection("松手即开始录制 · 再按快捷键结束 · Esc 取消", func(selection screenSelection) { a.startRecording(encoder, selection) })
}
