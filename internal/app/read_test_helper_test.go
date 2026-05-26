package app

import "fmt"

func expectedReadSingleLineOutput(line string) string {
	return expectedReadSingleLineOutputForPath("app.go", line)
}

func expectedReadSingleLineOutputForPath(path, line string) string {
	return expectedReadOutputForPath(path, fmt.Sprintf("1: %s\n(End of file - total 1 lines; shown lines 1-1)", line))
}

func expectedReadOutputForPath(path, body string) string {
	return fmt.Sprintf("<path>%s</path>\n<type>file</type>\n<content>\n%s\n</content>", path, body)
}
