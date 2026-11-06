package app

import (
	"errors"
	"fmt"
	"github.com/egoist/mygo"
	"os"
	"path/filepath"
	"strings"
)

type recordingSession struct {
	encoder                          string
	selection                        screenSelection
	segment                          *recordTask
	paths                            []string
	pausing, paused, finishing, keep bool
	elapsed                          int64
}

func (a *app) startRecording(encoder string, selection screenSelection) {
	s := &recordingSession{encoder: encoder, selection: selection}
	a.recordTask = s
	a.taskMessage = "正在启动录制…"
	if err := a.startRecordSegment(s); err != nil {
		a.recordTask = nil
		a.status = "无法开始录屏：" + err.Error()
		a.showPanel()
		return
	}
	a.showTaskWindow("区域录屏", selection.Bounds, func() { a.stopRecording(true) }, func() { a.stopRecording(false) })
	a.refreshTaskWindow()
}

func (a *app) toggleRecordingPause() {
	s := a.recordTask
	if s == nil || s.finishing || s.pausing {
		return
	}
	if s.paused {
		if err := a.startRecordSegment(s); err != nil {
			a.taskMessage = "恢复失败，可重试或结束"
		} else {
			s.paused = false
			a.taskMessage = "正在继续录制…"
		}
	} else if s.segment != nil && s.segment.frames.Load() > 0 {
		s.pausing = true
		a.taskMessage = "正在暂停…"
		s.segment.stop(true)
	}
	a.refreshTaskWindow()
}

func (a *app) stopRecording(keep bool) {
	s := a.recordTask
	if s == nil || s.finishing {
		return
	}
	s.finishing, s.keep = true, keep
	a.taskMessage = "正在取消录制…"
	if keep {
		a.taskMessage = "正在准备预览…"
	}
	if s.segment != nil {
		s.segment.stop(keep)
	} else {
		a.finalizeRecording(s, nil)
	}
	a.refreshTaskWindow()
}

func (a *app) startRecordSegment(s *recordingSession) error {
	var audioInputs [][]string
	stopAudio := func() {}
	if a.recordSystem {
		input, stop, err := startSystemAudio()
		if err != nil {
			return fmt.Errorf("无法录制系统声音：%w", err)
		}
		stopAudio = stop
		audioInputs = append(audioInputs, input)
	}
	if a.recordMic {
		audioInputs = append(audioInputs, []string{"-thread_queue_size", "128", "-f", "openal", "-channels", "1", "-sample_rate", "48000", "-analyzeduration", "1", "-probesize", "32", "-i", ""})
	}
	var segment *recordTask
	segment, err := startRecorder(s.encoder, s.selection.Bounds, filepath.Join(videoDirectory(), ".drafts"), audioInputs, func(frames int64) {
		mygo.RunOnMain(func() {
			if a.recordTask != s || s.segment != segment || frames < 1 || s.pausing || s.finishing {
				return
			}
			seconds := (s.elapsed + frames) / 30
			a.taskMessage = fmt.Sprintf("录屏中  %02d:%02d", seconds/60, seconds%60)
			a.refreshTaskWindow()
		})
	}, func(path string, err error) {
		stopAudio()
		mygo.RunOnMain(func() {
			if a.recordTask != s {
				return
			}
			s.segment = nil
			if path != "" {
				s.elapsed += segment.frames.Load()
				s.paths = append(s.paths, path)
			}
			if s.finishing {
				a.finalizeRecording(s, err)
				return
			}
			if s.pausing || len(s.paths) > 0 {
				s.pausing, s.paused = false, true
				a.taskMessage = "已暂停"
				if err != nil {
					a.taskMessage = "本段录制失败，可重试或结束"
				}
				a.refreshTaskWindow()
				return
			}
			s.finishing, s.keep = true, true
			a.finalizeRecording(s, err)
		})
	})
	if err != nil {
		stopAudio()
		return err
	}
	s.segment = segment
	return nil
}

func (a *app) finalizeRecording(s *recordingSession, segmentErr error) {
	var audio []string
	if a.recordSystem {
		audio = append(audio, "系统声音")
	}
	if a.recordMic {
		audio = append(audio, "麦克风")
	}
	go func() {
		var path string
		err := segmentErr
		if !s.keep {
			for _, part := range s.paths {
				_ = os.Remove(part)
			}
			err = errRecordingCancelled
		} else if len(s.paths) > 0 {
			path, err = joinRecordingSegments(s.encoder, s.paths)
		} else if err == nil {
			err = errors.New("没有录到可播放的画面")
		}
		mygo.RunOnMain(func() {
			a.recordTask = nil
			defer a.closeTaskWindow()
			if errors.Is(err, errRecordingCancelled) {
				a.quitAfterPreview = false
				a.status = "已取消录屏"
			} else if err != nil {
				a.quitAfterPreview = false
				a.status = err.Error()
				a.showPanel()
			} else {
				a.status = "录屏已结束，等待预览或保存"
				if a.tray != nil {
					a.tray.SetToolTip("映刻 · 录屏待保存")
				}
				if err := a.showRecordPreview(path, strings.Join(audio, " + ")); err != nil {
					a.quitAfterPreview = false
					a.status = fmt.Sprintf("无法打开录屏预览，草稿保留在 %s：%v", path, err)
					a.showPanel()
				}
			}
			a.window.Invalidate()
		})
	}()
}
