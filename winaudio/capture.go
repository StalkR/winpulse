//go:build windows
// +build windows

// Package winaudio implements audio capture on Windows.
package winaudio

import (
	"context"
	"fmt"
	"io"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	wca "github.com/moutend/go-wca"
	"golang.org/x/sys/windows"
)

// Capture captures local audio and writes it to the sink.
// Format is float32 little-endian, stereo 16-bit 44100 Hz (CD Quality).
func Capture(ctx context.Context, sink io.Writer) error {
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		return fmt.Errorf("winaudio: CoInitializeEx: %v", err)
	}
	defer ole.CoUninitialize()

	var de *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &de); err != nil {
		return fmt.Errorf("winaudio: CoCreateInstance: %v", err)
	}
	defer de.Release()

	var mmd *wca.IMMDevice
	if err := de.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &mmd); err != nil {
		return fmt.Errorf("winaudio: GetDefaultAudioEndpoint: %v", err)
	}
	defer mmd.Release()

	var ac *wca.IAudioClient
	if err := mmd.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
		return fmt.Errorf("winaudio: Activate: %v", err)
	}
	defer ac.Release()

	var wfx *wca.WAVEFORMATEX
	if err := ac.GetMixFormat(&wfx); err != nil {
		return fmt.Errorf("winaudio: GetMixFormat: %v", err)
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	const WaveFormatExtensible = 0xfffe // float32 little-endian
	if wfx.WFormatTag != WaveFormatExtensible {
		return fmt.Errorf("winaudio: expected WaveFormatExtensible but got %v", wfx.WFormatTag)
	}
	if channels := uint16(2); wfx.NChannels != channels {
		return fmt.Errorf("winaudio: expected %v channels but got %v", channels, wfx.NChannels)
	}
	if bits := uint16(16); wfx.WBitsPerSample/wfx.NChannels != bits {
		return fmt.Errorf("winaudio: expected %v-bit but got %v-bit", bits, wfx.WBitsPerSample)
	}
	if freq := uint32(44100); wfx.NSamplesPerSec != freq {
		return fmt.Errorf("winaudio: expected %v Hz but got %v Hz", freq, wfx.NSamplesPerSec)
	}

	var defaultPeriod wca.REFERENCE_TIME
	var minimumPeriod wca.REFERENCE_TIME
	if err := ac.GetDevicePeriod(&defaultPeriod, &minimumPeriod); err != nil {
		return fmt.Errorf("winaudio: GetDevicePeriod: %v", err)
	}
	latency := time.Duration(int(defaultPeriod) * 100)

	const hnsBufferDuration = 0
	const hnsPeriodicity = 0
	if err := ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, wca.AUDCLNT_STREAMFLAGS_LOOPBACK, hnsBufferDuration, hnsPeriodicity, wfx, nil); err != nil {
		return fmt.Errorf("winaudio: Initialize: %v", err)
	}

	var acc *wca.IAudioCaptureClient
	if err := ac.GetService(wca.IID_IAudioCaptureClient, &acc); err != nil {
		return fmt.Errorf("winaudio: GetService: %v", err)
	}
	defer acc.Release()

	// register with MMCSS
	revertMm, err := setMmThreadCharacteristics("Audio")
	if err != nil {
		return fmt.Errorf("winaudio: setMmThreadCharacteristics: %v", err)
	}
	defer revertMm()

	if err := ac.Start(); err != nil {
		return fmt.Errorf("winaudio: Start: %v", err)
	}
	defer ac.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		for {
			var packetLength uint32
			if err := acc.GetNextPacketSize(&packetLength); err != nil {
				return fmt.Errorf("winaudio: GetNextPacketSize: %v", err)
			}
			if packetLength == 0 {
				break
			}

			var data *byte
			var numFramesAvailable uint32
			var flags uint32
			if err := acc.GetBuffer(&data, &numFramesAvailable, &flags, nil, nil); err != nil {
				return fmt.Errorf("winaudio: GetBuffer: %v", err)
			}
			// now careful not to return/break early and skipping ReleaseBuffer

			length := int(numFramesAvailable) * int(wfx.NBlockAlign)
			var sl = struct {
				addr *byte
				len  int
				cap  int
			}{data, length, length}
			b := *(*[]byte)(unsafe.Pointer(&sl))

			if flags&wca.AUDCLNT_BUFFERFLAGS_SILENT != 0 {
				// must ignore values and treat it as silence
				// memset zero, rely on compiler to optimize
				for i := range b {
					b[i] = 0
				}
			}

			_, writeErr := sink.Write(b)

			if err := acc.ReleaseBuffer(numFramesAvailable); err != nil {
				return fmt.Errorf("winaudio: ReleaseBuffer: %v", err)
			}

			if writeErr != nil {
				return fmt.Errorf("winaudio: write: %v", err)
			}
		}
		time.Sleep(latency / 2)
	}
}

var (
	modavrt                             = windows.MustLoadDLL("avrt.dll")
	procAvSetMmThreadCharacteristicsA   = modavrt.MustFindProc("AvSetMmThreadCharacteristicsA")
	procAvRevertMmThreadCharacteristics = modavrt.MustFindProc("AvRevertMmThreadCharacteristics")
)

func setMmThreadCharacteristics(name string) (func() error, error) {
	taskName, err := windows.BytePtrFromString(name)
	if err != nil {
		return nil, err
	}
	taskIndex := 0
	avHandle, _, _ := procAvSetMmThreadCharacteristicsA.Call(uintptr(unsafe.Pointer(taskName)), uintptr(unsafe.Pointer(&taskIndex)))
	if avHandle == 0 {
		return nil, fmt.Errorf("winaudio: AvSetMmThreadCharacteristicsA: %v", avHandle)
	}
	return func() error {
		r, _, _ := procAvRevertMmThreadCharacteristics.Call(avHandle)
		if r == 0 {
			return fmt.Errorf("winaudio: AvRevertMmThreadCharacteristics: %v", r)
		}
		return nil
	}, nil
}
