package provider

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

type openAISSEPacket struct {
	Name string
	Data []byte
}

func readOpenAISSEPacket(reader *bufio.Reader) (openAISSEPacket, error) {
	var packet openAISSEPacket
	var dataLines []string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				line = strings.TrimRight(line, "\r\n")
				if line != "" {
					appendSSELine(line, &packet, &dataLines)
				}
				if packet.Name == "" && len(dataLines) == 0 {
					return openAISSEPacket{}, io.EOF
				}
				packet.Data = []byte(strings.Join(dataLines, "\n"))
				return packet, nil
			}
			return openAISSEPacket{}, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if packet.Name == "" && len(dataLines) == 0 {
				continue
			}
			packet.Data = []byte(strings.Join(dataLines, "\n"))
			return packet, nil
		}
		appendSSELine(line, &packet, &dataLines)
	}
}

func appendSSELine(line string, packet *openAISSEPacket, dataLines *[]string) {
	switch {
	case strings.HasPrefix(line, ":"):
		return
	case strings.HasPrefix(line, "event:"):
		packet.Name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
	case strings.HasPrefix(line, "data:"):
		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimPrefix(data, " ")
		*dataLines = append(*dataLines, data)
	}
}
