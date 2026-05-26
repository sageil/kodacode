package app

import (
	"io"
	"os"
)

func IsInteractiveTerminal(in io.Reader, out io.Writer) bool {
	return isTTYDevice(in) && isTTYDevice(out)
}

func isTTYDevice(value any) bool {
	file, ok := value.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
