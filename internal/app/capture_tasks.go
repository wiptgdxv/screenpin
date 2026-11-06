package app

import (
	"bytes"
	"errors"
	"fmt"
	"image/png"
	"sync"
	"time"

	"screenpin/internal/scroll"

	"github.com/egoist/mygo"
)

type scrollTask struct {
	stop chan bool
	once sync.Once
	undo chan struct{}
}

func (s *scrollTask) finish(keep bool) { s.once.Do(func() { s.stop <- keep }) }

func (a *app) beginSelection(hint string, start func(screenSelection)) {
	if a.capturing || a.scrollTask != nil || a.recordTask != nil || a.quitting {
		a.status = "请先完成当前截图或录屏任务"
		return
	}
	a.capturing = true
	wasVisible := a.window.IsVisible()
	a.window.Hide()
	go func() {
		time.Sleep(180 * time.Millisecond)
		selection, err := selectScreen(hint)
		mygo.RunOnMain(func() {
			a.capturing = false
			if a.quitting {
				return
			}
			if err != nil {
				a.status = err.Error()
				if !errors.Is(err, errCaptureCancelled) || wasVisible {
					a.window.Show()
				}
				a.window.Invalidate()
				return
			}
			start(selection)
		})
	}()
}

func (a *app) scrollCapture() {
	if a.scrollTask != nil {
		a.scrollTask.finish(true)
		return
	}
	a.beginSelection("框选滚动正文，避开固定页头 · Esc 取消", a.startScroll)
}

func (a *app) startScroll(selection screenSelection) {
	if min(selection.Bounds.Dy(), selection.Bounds.Dx()) < 40 || max(selection.Bounds.Dy(), selection.Bounds.Dx()) < 80 {
		a.status = "滚动选区至少需要 40 × 80 像素"
		a.window.Show()
		return
	}
	task := &scrollTask{stop: make(chan bool, 1), undo: make(chan struct{}, 1)}
	a.scrollTask = task
	a.taskMessage = "滚动即可识别方向"
	a.status = "滚动截图中，再按滚动截图快捷键完成"
	a.showTaskWindow("滚动截图", selection.Bounds, func() { task.finish(true) }, func() { task.finish(false) })
	go func() {
		composer := scroll.New(selection.Image)
		awaitingReturn := false
		lastMessage := ""
		report := func(message string) {
			if message == lastMessage {
				return
			}
			lastMessage = message
			mygo.RunOnMain(func() {
				if a.scrollTask == task {
					a.taskMessage = message
					a.refreshTaskWindow()
				}
			})
		}
		ticker := time.NewTicker(55 * time.Millisecond)
		defer ticker.Stop()
		var finalErr error
		keep := false
	loop:
		for {
			select {
			case keep = <-task.stop:
				// A quick stop can arrive before the next periodic sample.
				// Keep the assembled image if the last frame cannot be joined.
				if keep && !awaitingReturn {
					if frame, err := grabScreenRegion(selection.Bounds); err == nil {
						_ = composer.Add(frame)
					}
				}
				break loop
			case <-task.undo:
				if composer.Undo() {
					awaitingReturn = true
					report("已撤回 · 滚回上一段")
				}
			case <-ticker.C:
				frame, err := grabScreenRegion(selection.Bounds)
				if err != nil {
					finalErr = err
					break loop
				}
				if awaitingReturn {
					if !composer.AlignAfterUndo(frame) {
						continue
					}
					awaitingReturn = false
					report("已对齐，可继续滚动")
					continue
				}
				err = composer.Add(frame)
				if errors.Is(err, scroll.ErrLimit) {
					keep = true
					break loop
				}
				message := "滚动即可识别方向"
				if err != nil {
					message = err.Error()
				} else if horizontal, known := composer.Axis(); known {
					size := composer.Size()
					axis := "上下拼接"
					if horizontal {
						axis = "左右拼接"
					}
					message = fmt.Sprintf("%d × %d · %s", size.X, size.Y, axis)
				}
				report(message)
			}
		}
		var encoded bytes.Buffer
		var resultImage = selection.Image
		if keep {
			resultImage = composer.Image()
			finalErr = png.Encode(&encoded, resultImage)
		}
		mygo.RunOnMain(func() {
			if a.scrollTask != task {
				return
			}
			a.scrollTask = nil
			a.closeTaskWindow()
			if a.quitting {
				return
			}
			if finalErr != nil {
				a.status = "滚动截图失败：" + finalErr.Error()
				a.window.Show()
			} else if keep {
				a.publishImage(resultImage, encoded.Bytes(), "长截图_"+time.Now().Format("20060102_150405")+".png")
			} else {
				a.status = "已取消滚动截图，剪贴板未改变"
			}
			a.window.Invalidate()
		})
	}()
}
