package tool

import (
	"testing"
)

func TestFlexUnmarshal_strictPass(t *testing.T) {
	var p struct {
		Name    string  `json:"name"`
		Count   int     `json:"count"`
		Rate    float64 `json:"rate"`
		Enabled bool    `json:"enabled"`
	}
	err := flexUnmarshal([]byte(`{"name":"x","count":5,"rate":1.5,"enabled":true}`), &p)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "x" || p.Count != 5 || p.Rate != 1.5 || !p.Enabled {
		t.Fatalf("unexpected: %+v", p)
	}
}

func TestFlexUnmarshal_stringWrappedNumber(t *testing.T) {
	var p struct {
		Offset int     `json:"offset"`
		Rate   float64 `json:"rate"`
	}
	err := flexUnmarshal([]byte(`{"offset":"42","rate":"3.14"}`), &p)
	if err != nil {
		t.Fatal(err)
	}
	if p.Offset != 42 {
		t.Fatalf("offset = %d, want 42", p.Offset)
	}
	if p.Rate != 3.14 {
		t.Fatalf("rate = %f, want 3.14", p.Rate)
	}
}

func TestFlexUnmarshal_stringWrappedBool(t *testing.T) {
	var p struct {
		Enabled bool `json:"enabled"`
	}
	err := flexUnmarshal([]byte(`{"enabled":"true"}`), &p)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Enabled {
		t.Fatal("expected true")
	}
}

func TestFlexUnmarshal_stringWrappedArray(t *testing.T) {
	type item struct {
		ID string `json:"id"`
	}
	var p struct {
		Items []item `json:"items"`
	}
	err := flexUnmarshal([]byte(`{"items":"[{\"id\":\"a\"},{\"id\":\"b\"}]"}`), &p)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 2 || p.Items[0].ID != "a" || p.Items[1].ID != "b" {
		t.Fatalf("unexpected: %+v", p.Items)
	}
}

func TestFlexUnmarshal_singleObjectToSlice(t *testing.T) {
	type item struct {
		ID string `json:"id"`
	}
	var p struct {
		Items []item `json:"items"`
	}
	err := flexUnmarshal([]byte(`{"items":{"id":"solo"}}`), &p)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 1 || p.Items[0].ID != "solo" {
		t.Fatalf("unexpected: %+v", p.Items)
	}
}

func TestFlexUnmarshal_singlePrimitiveToSlice(t *testing.T) {
	var p struct {
		Files []string `json:"files"`
	}
	err := flexUnmarshal([]byte(`{"files":"src/main.go"}`), &p)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 1 || p.Files[0] != "src/main.go" {
		t.Fatalf("unexpected: %+v", p.Files)
	}
}

func TestFlexUnmarshal_pointerInt(t *testing.T) {
	var p struct {
		Depth *int `json:"depth"`
	}
	err := flexUnmarshal([]byte(`{"depth":"4"}`), &p)
	if err != nil {
		t.Fatal(err)
	}
	if p.Depth == nil || *p.Depth != 4 {
		t.Fatalf("depth = %v, want *4", p.Depth)
	}
}

func TestFlexUnmarshal_mixedCorrectAndWrong(t *testing.T) {
	var p struct {
		Name    string `json:"name"`
		Timeout int    `json:"timeout"`
		Debug   bool   `json:"debug"`
	}
	// name is correct string, timeout is string-wrapped, debug is string-wrapped
	err := flexUnmarshal([]byte(`{"name":"test","timeout":"30","debug":"false"}`), &p)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "test" || p.Timeout != 30 || p.Debug {
		t.Fatalf("unexpected: %+v", p)
	}
}
