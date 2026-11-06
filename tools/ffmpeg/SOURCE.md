# FFmpeg 运行依赖

版本：`9.0.2-essentials_build-www.gyan.dev`，Windows x64 静态构建。

- FFmpeg 官方下载入口：[ffmpeg.org/download.html](https://ffmpeg.org/download.html)，其中链接到 Gyan 的 Windows 构建。
- 构建发布页：[gyan.dev/ffmpeg/builds](https://www.gyan.dev/ffmpeg/builds/)。
- 下载归档：`ffmpeg-release-essentials.7z`。
- SHA-256：`4705843ccaaf54257c16ad90f3e952ece33c17df964ecf7bfdbb0f49c7171077`，已与发布方 `.sha256` 文件核对。
- 对应源代码：[FFmpeg commit 946fcce07b](https://github.com/FFmpeg/FFmpeg/commit/946fcce07b)。
- 许可证与构建说明随附在 `LICENSE` 和 `README.txt` 中。

应用通过独立进程调用 `ffmpeg.exe` 录制 MP4；`ffprobe.exe` 用于媒体验证。移动应用时请一并保留 `tools/ffmpeg` 目录。本轮未修改 FFmpeg 二进制，也未配置全局 PATH 或系统软件安装。
