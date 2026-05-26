package provider

import "fmt"

func errUnsupportedReasoningVariant(model ModelRef, variant string) error {
	return fmt.Errorf("unsupported reasoning variant %q for %s", variant, model.String())
}
