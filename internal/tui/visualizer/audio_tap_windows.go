//go:build windows

package visualizer

import (
	"runtime"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

func newDarwinAudioTap() *AudioTap {
	return nil
}

// wasapiTap is the Windows WASAPI implementation
type wasapiTap struct {
	audioClient *wca.IAudioClient
	capture     *wca.IAudioCaptureClient
	buf         *ringBuffer
	done        chan struct{}
	closed      bool
}

func wasapiTapReadLoop(tap *wasapiTap) {
	for {
		select {
		case <-tap.done:
			return
		default:
		}

		var frames uint32
		if err := tap.capture.GetNextPacketSize(&frames); err != nil {
			if audioLogger != nil {
				audioLogger.Printf("AudioTap: GetNextPacketSize error: %v", err)
			}
			return
		}

		if frames == 0 {
			runtime.Gosched()
			continue
		}

		var data *byte
		var flags uint32
		var devicePos, qpcPos uint64
		if err := tap.capture.GetBuffer(&data, &frames, &flags, &devicePos, &qpcPos); err != nil {
			if audioLogger != nil {
				audioLogger.Printf("AudioTap: GetBuffer error: %v", err)
			}
			continue
		}

		if data != nil && frames > 0 {
			samples := unsafe.Slice((*float32)(unsafe.Pointer(data)), frames)
			tap.buf.Write(samples)
		}

		if err := tap.capture.ReleaseBuffer(frames); err != nil {
			if audioLogger != nil {
				audioLogger.Printf("AudioTap: ReleaseBuffer error: %v", err)
			}
		}
	}
}

// Close stops the WASAPI stream and releases COM resources.
func (wt *wasapiTap) Close() {
	if wt.closed {
		return
	}
	wt.closed = true

	if wt.audioClient != nil {
		_ = wt.audioClient.Stop()
	}
	if wt.capture != nil {
		wt.capture.Release()
	}
	if wt.audioClient != nil {
		wt.audioClient.Release()
	}
}

// winWASAPITap creates WASAPI loopback audio tap
func winWASAPITap() *wasapiTap {
	if audioLogger != nil {
		audioLogger.Printf("AudioTap: initializing Windows WASAPI loopback")
	}

	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: COM init: %v", err)
		}
	}

	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &enumerator); err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: enumerator: %v", err)
		}
		return nil
	}
	defer enumerator.Release()

	var device *wca.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &device); err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: endpoint: %v", err)
		}
		return nil
	}
	defer device.Release()

	var audioClient *wca.IAudioClient
	if err := device.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &audioClient); err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: activate: %v", err)
		}
		return nil
	}
	var format *wca.WAVEFORMATEX
	if err := audioClient.GetMixFormat(&format); err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: get format: %v", err)
		}
		return nil
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(format)))

	if audioLogger != nil {
		audioLogger.Printf("AudioTap: format: ch=%d rate=%d",
			format.NChannels, format.NSamplesPerSec)
	}

	err := audioClient.Initialize(
		wca.AUDCLNT_SHAREMODE_SHARED,
		wca.AUDCLNT_STREAMFLAGS_LOOPBACK,
		wca.REFERENCE_TIME(50*10000),
		0,
		format,
		nil,
	)
	if err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: init: %v", err)
		}
		return nil
	}

	var capture *wca.IAudioCaptureClient
	if err := audioClient.GetService(wca.IID_IAudioCaptureClient, &capture); err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: service: %v", err)
		}
		return nil
	}

	if err := audioClient.Start(); err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: start: %v", err)
		}
		return nil
	}

	wTap := &wasapiTap{
		audioClient: audioClient,
		capture:     capture,
		buf:         newRingBuffer(8192),
		done:        make(chan struct{}),
	}

	go wasapiTapReadLoop(wTap)

	if audioLogger != nil {
		audioLogger.Printf("AudioTap: WASAPI started")
	}

	return wTap
}

// newWASAPITap returns a Windows WASAPI loopback audio tap.
func newWASAPITap() *AudioTap {
	wtap := winWASAPITap()
	if wtap == nil {
		return nil
	}
	return &AudioTap{
		buf:    wtap.buf,
		done:   wtap.done,
		closed: false,
		cleanup: func() {
			wtap.Close()
		},
	}
}

// WASAPIAvailable checks WASAPI availability
func WASAPIAvailable() bool {
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		// CO_E_ALREADYINITIALIZED = 0x80010106
	}

	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &enumerator); err != nil {
		return false
	}
	defer enumerator.Release()

	var device *wca.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &device); err != nil {
		return false
	}
	defer device.Release()

	var audioClient *wca.IAudioClient
	return device.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &audioClient) == nil
}
