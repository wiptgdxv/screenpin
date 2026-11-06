//go:build windows

package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"time"
	"unsafe"

	"github.com/degubites/go-wca/pkg/wca"
	"github.com/go-ole/go-ole"
)

const (
	systemSampleRate = 48000
	systemFrameBytes = 4 // Stereo, signed 16-bit PCM.
)

func startSystemAudio() ([]string, func(), error) {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	stop := func() { cancel(); _ = listener.Close() }
	ready := make(chan error, 1)
	go func() {
		defer listener.Close()
		captureSystemAudio(ctx, listener, ready)
	}()
	if err := <-ready; err != nil {
		stop()
		return nil, nil, err
	}
	args := []string{"-thread_queue_size", "128", "-f", "s16le", "-ar", "48000", "-ac", "2", "-analyzeduration", "1", "-probesize", "32", "-i", "tcp://" + listener.Addr().String()}
	return args, stop, nil
}

func captureSystemAudio(ctx context.Context, listener *net.TCPListener, ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		var result *ole.OleError
		if !errors.As(err, &result) || result.Code() != 1 { // S_FALSE also initializes COM.
			ready <- err
			return
		}
	}
	defer ole.CoUninitialize()

	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, ole.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &enumerator); err != nil {
		ready <- err
		return
	}
	defer enumerator.Release()
	var output *wca.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EMultimedia, &output); err != nil {
		ready <- fmt.Errorf("找不到默认播放设备：%w", err)
		return
	}
	defer output.Release()
	var client *wca.IAudioClient
	if err := output.Activate(wca.IID_IAudioClient, ole.CLSCTX_ALL, nil, &client); err != nil {
		ready <- err
		return
	}
	defer client.Release()
	format := wca.WAVEFORMATEX{WFormatTag: 1, NChannels: 2, NSamplesPerSec: systemSampleRate, NAvgBytesPerSec: systemSampleRate * systemFrameBytes, NBlockAlign: systemFrameBytes, WBitsPerSample: 16}
	flags := uint32(wca.AUDCLNT_STREAMFLAGS_LOOPBACK | wca.AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM | wca.AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY)
	if err := client.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, flags, 0, 0, &format, nil); err != nil {
		ready <- err
		return
	}
	var capture *wca.IAudioCaptureClient
	if err := client.GetService(wca.IID_IAudioCaptureClient, &capture); err != nil {
		ready <- err
		return
	}
	defer capture.Release()
	if err := client.Start(); err != nil {
		ready <- err
		return
	}
	defer client.Stop()
	ready <- nil

	_ = listener.SetDeadline(time.Now().Add(8 * time.Second))
	conn, err := listener.AcceptTCP()
	if err != nil {
		return
	}
	_ = listener.Close()
	defer conn.Close()
	streamSystemAudio(ctx, conn, capture)
}

func streamSystemAudio(ctx context.Context, conn *net.TCPConn, capture *wca.IAudioCaptureClient) {
	const interval = 10 * time.Millisecond
	const packetBytes = systemSampleRate * int(interval) / int(time.Second) * systemFrameBytes
	pcm := make([]byte, packetBytes)
	var pending []byte
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		for {
			var next uint32
			if err := capture.GetNextPacketSize(&next); err != nil {
				return
			}
			if next == 0 {
				break
			}
			var data *byte
			var frames, flags uint32
			if err := capture.GetBuffer(&data, &frames, &flags, nil, nil); err != nil {
				return
			}
			count := int(frames) * systemFrameBytes
			if data == nil || flags&wca.AUDCLNT_BUFFERFLAGS_SILENT != 0 {
				pending = append(pending, make([]byte, count)...)
			} else {
				pending = append(pending, unsafe.Slice(data, count)...)
			}
			if err := capture.ReleaseBuffer(frames); err != nil {
				return
			}
		}
		// The endpoint may deliver no packets while nothing is playing.
		// Keep the encoder clock moving and represent these gaps as silence.
		clear(pcm)
		if len(pending) > 5*packetBytes {
			pending = pending[len(pending)-3*packetBytes:]
		}
		copied := copy(pcm, pending)
		pending = pending[copied:]
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if _, err := conn.Write(pcm); err != nil {
			return
		}
	}
}
