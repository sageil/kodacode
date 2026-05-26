package events

import "time"

const jsonTimeLayout = time.RFC3339Nano

func parseJSONTime(raw string) (time.Time, error) {
	return time.Parse(jsonTimeLayout, raw)
}
