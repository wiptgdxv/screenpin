//go:build windows

package ocr

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"screenpin/internal/process"
)

func recognizeWeChatText(img image.Image) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	root := filepath.Dir(executable)
	helperDir := filepath.Join(root, "tools", "wechat-ocr")
	helper := filepath.Join(helperDir, "Screenpin.WeChatOcr.exe")
	if filepath.Base(root) == "artifacts" {
		helperDir = filepath.Join(filepath.Dir(root), "tools", "wechat-ocr")
		helper = filepath.Join(helperDir, "Screenpin.WeChatOcr.exe")
	}
	if _, err := os.Stat(helper); err != nil {
		return "", fmt.Errorf("微信 OCR 辅助程序未安装")
	}
	dataDir := helperDir
	if _, err := os.Stat(filepath.Join(dataDir, "wco_data", "WeChatOCR.exe")); err != nil {
		dataDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "STranslate", "current", "Plugins", "STranslate.Plugin.Ocr.WeChatBuiltIn")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "wco_data", "WeChatOCR.exe")); err != nil {
		return "", fmt.Errorf("未找到微信 OCR 数据，请安装带内置微信 OCR 的 STranslate")
	}
	file, err := os.CreateTemp("", "screenpin-wechat-*.png")
	if err != nil {
		return "", err
	}
	path := file.Name()
	_ = file.Close()
	defer os.Remove(path)
	if err := writeOCRImage(path, img); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, helper, path, dataDir)
	cmd.WaitDelay = 2 * time.Second
	process.HideWindow(cmd)
	// The bridge launches an OCR worker; shut down both on timeout or exit.
	var output, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &output, &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	release, err := process.Guard(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", err
	}
	defer release()
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("微信 OCR 识别超时")
		}
		reason := strings.TrimSpace(stderr.String())
		if reason == "" {
			reason = err.Error()
		}
		return "", fmt.Errorf("微信 OCR 识别失败：%s", reason)
	}
	return strings.TrimSpace(output.String()), nil
}
