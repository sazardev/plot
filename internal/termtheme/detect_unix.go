//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly || solaris

package termtheme

import (
	"fmt"
	"image/color"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

var oscResponse = regexp.MustCompile(`\x1b\](\d+)(?:;(\d+))?;([^\x07\x1b]*)(?:\x07|\x1b\\)`)

// Detect asks a real terminal for its foreground, background and 16-color ANSI
// palette using OSC 4/10/11. It returns false when the terminal does not answer
// or the descriptors are not a TTY, in which case the caller should use
// DefaultPalette. The terminal is restored before returning.
func Detect(in, out *os.File, timeout time.Duration) (Palette, bool) {
	palette := DefaultPalette()
	if in == nil || out == nil {
		return palette, false
	}
	inputFD := int(in.Fd())
	if !term.IsTerminal(inputFD) || !term.IsTerminal(int(out.Fd())) {
		return palette, false
	}
	if timeout <= 0 {
		timeout = 120 * time.Millisecond
	}
	state, err := term.MakeRaw(inputFD)
	if err != nil {
		return palette, false
	}
	defer func() { _ = term.Restore(inputFD, state) }()

	for index := 0; index < 16; index++ {
		_, _ = fmt.Fprintf(out, "\x1b]4;%d;?\x1b\\", index)
	}
	_, _ = fmt.Fprint(out, "\x1b]10;?\x1b\\\x1b]11;?\x1b\\")

	deadline := time.Now().Add(timeout)
	var responses strings.Builder
	for time.Now().Before(deadline) {
		if !readable(inputFD, time.Until(deadline)) {
			break
		}
		buffer := make([]byte, 256)
		n, err := in.Read(buffer)
		if err != nil || n == 0 {
			break
		}
		responses.Write(buffer[:n])
		text := responses.String()
		if strings.Count(text, "\x1b]") >= 18 && strings.Contains(text, "]11;") {
			break
		}
	}
	return parseResponses(responses.String(), palette)
}

func readable(fd int, timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}
	tv := unix.NsecToTimeval(timeout.Nanoseconds())
	for {
		var readfds unix.FdSet
		readfds.Set(fd)
		n, err := unix.Select(fd+1, &readfds, nil, nil, &tv)
		if err == unix.EINTR {
			continue
		}
		return err == nil && n > 0
	}
}

func parseResponses(response string, palette Palette) (Palette, bool) {
	detected := false
	for _, match := range oscResponse.FindAllStringSubmatch(response, -1) {
		code, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		index := -1
		if match[2] != "" {
			index, err = strconv.Atoi(match[2])
			if err != nil {
				continue
			}
		}
		value, ok := parseColor(match[3])
		if !ok {
			continue
		}
		switch code {
		case 4:
			if index >= 0 && index < 16 {
				palette.ANSI[index] = value
				detected = true
			}
		case 10:
			palette.Foreground = value
			detected = true
		case 11:
			palette.Background = value
			detected = true
		}
	}
	return palette, detected
}

func parseColor(value string) (color.RGBA, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "#") && len(value) == 7 {
		parsed, err := strconv.ParseUint(value[1:], 16, 32)
		if err != nil {
			return color.RGBA{}, false
		}
		return color.RGBA{
			R: uint8(parsed >> 16),
			G: uint8(parsed >> 8),
			B: uint8(parsed),
			A: 255,
		}, true
	}
	if strings.HasPrefix(value, "rgb:") || strings.HasPrefix(value, "rgba:") {
		parts := strings.SplitN(value, ":", 2)
		if len(parts) != 2 {
			return color.RGBA{}, false
		}
		components := strings.Split(parts[1], "/")
		if len(components) < 3 {
			return color.RGBA{}, false
		}
		r, okR := parseComponent(components[0])
		g, okG := parseComponent(components[1])
		b, okB := parseComponent(components[2])
		if !okR || !okG || !okB {
			return color.RGBA{}, false
		}
		return color.RGBA{R: r, G: g, B: b, A: 255}, true
	}
	return color.RGBA{}, false
}

func parseComponent(value string) (uint8, bool) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 4 {
		return 0, false
	}
	parsed, err := strconv.ParseUint(value, 16, 16)
	if err != nil {
		return 0, false
	}
	var max uint64 = 1
	for range value {
		max *= 16
	}
	return uint8((parsed*255 + (max-1)/2) / (max - 1)), true
}
