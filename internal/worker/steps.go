package worker

import (
	"context"

	"journaletto/internal/provider"
)

// DefaultSteps is the processing pipeline. T03-T07 append their stages here.
func DefaultSteps(p provider.Provider) []Step {
	return []Step{{
		Name: "embed",
		Run: func(ctx context.Context, e Entry) error {
			_, err := p.Embed(ctx, e.Title+"\n\n"+e.Body)
			return err
		},
	}}
}
