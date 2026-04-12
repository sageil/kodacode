package pipeline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sageil/kodacode/v1/internal/pipeline"
)

func TestChain_ExecutesInOrder(t *testing.T) {
	var order []int
	m1 := func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		order = append(order, 1)
		err := next(ctx, req)
		order = append(order, 4)
		return err
	}
	m2 := func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		order = append(order, 2)
		err := next(ctx, req)
		order = append(order, 3)
		return err
	}
	chain := pipeline.BuildChain(m1, m2)
	req := &pipeline.TurnRequest{SessionID: "s1"}
	if err := chain.Execute(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 4}
	for i, v := range order {
		if v != want[i] {
			t.Errorf("order[%d] = %d, want %d", i, v, want[i])
		}
	}
}

func TestChain_StopsOnError(t *testing.T) {
	called := false
	boom := errors.New("boom")
	m1 := func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		return boom
	}
	m2 := func(ctx context.Context, req *pipeline.TurnRequest, next pipeline.TurnHandler) error {
		called = true
		return next(ctx, req)
	}
	chain := pipeline.BuildChain(m1, m2)
	err := chain.Execute(context.Background(), &pipeline.TurnRequest{})
	if !errors.Is(err, boom) {
		t.Errorf("chain.Execute() = %v, want boom error", err)
	}
	if called {
		t.Error("m2 should not have been called")
	}
}

func TestChain_EmptyChain(t *testing.T) {
	chain := pipeline.BuildChain()
	if err := chain.Execute(context.Background(), &pipeline.TurnRequest{}); err != nil {
		t.Errorf("empty chain Execute() = %v, want nil", err)
	}
}
