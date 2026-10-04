package workload

import (
	"context"
	"errors"
	"fmt"

	weir "github.com/batchstream/weir-go"
)

func (p *weirPath) ExecuteBatch(ctx context.Context, operations []Operation) []Outcome {
	if len(operations) == 1 {
		outcomes := []Outcome{p.Execute(ctx, operations[0])}
		return outcomes
	}
	if err := validateBatch(operations); err != nil {
		return batchFailure(operations, err)
	}
	if p.dataset.Config.LuaMutations && operations[0].Write {
		err := errors.New("Lua comparison requires one mutation per client call")
		outcomes := make([]Outcome, len(operations))
		for index := range outcomes {
			outcomes[index] = failed(err, false)
		}
		return outcomes
	}
	if operations[0].Write {
		requests := make([]*weir.MutateRequest, len(operations))
		for index, operation := range operations {
			document := &weir.Document{ContentType: p.dataset.ContentType(), Data: p.dataset.Document(operation)}
			request := &weir.MutateRequest{Resource: p.dataset.Resource(operation.Record), Action: weir.MutationPut, Document: document}
			requests[index] = request
		}
		opts := weir.MutateOptions{StoreName: p.dataset.Config.StoreName, Requests: requests}
		replies, err := p.client.Mutate(ctx, opts)
		if len(replies) != len(operations) {
			return batchFailure(operations, errors.Join(errors.New("Weir Mutate result count mismatch"), err))
		}
		outcomes := make([]Outcome, len(operations))
		for index, reply := range replies {
			outcomes[index] = mutationOutcome(reply, len(requests[index].Document.Data), err)
		}
		return outcomes
	}
	requests := make([]*weir.ReadRequest, len(operations))
	for index, operation := range operations {
		request := &weir.ReadRequest{Resource: p.dataset.Resource(operation.Record)}
		requests[index] = request
	}
	opts := weir.ReadOptions{StoreName: p.dataset.Config.StoreName, Requests: requests}
	replies, err := p.client.Read(ctx, opts)
	if len(replies) != len(operations) {
		return batchFailure(operations, errors.Join(errors.New("Weir Read result count mismatch"), err))
	}
	outcomes := make([]Outcome, len(operations))
	for index, reply := range replies {
		outcomes[index] = p.readOutcome(reply, operations[index], err)
	}
	return outcomes
}

// A confirmed APPLIED item can also contain a post-write business failure.
// Preserve that evidence, but exclude it from successful throughput. A failed
// stream retains confirmed item evidence; only unconfirmed writes remain unknown.
func mutationOutcome(result *weir.MutationResult, requestBytes int, rpcErr error) Outcome {
	if result != nil && result.GetOutcome() == weir.MutationApplied {
		outcome := Outcome{Status: Success, Applied: true, RequestBytes: uint64(requestBytes)}
		if result.GetFailure() != nil {
			outcome.Status = Failed
			failure := fmt.Errorf("Weir APPLIED with failure: %v", result.GetFailure())
			outcome.Error = failure.Error()
		}
		return outcome
	}
	if rpcErr == nil {
		rpcErr = fmt.Errorf("Weir mutation outcome=%v failure=%v", result.GetOutcome(), result.GetFailure())
	}
	return failed(rpcErr, result == nil || result.GetOutcome() == weir.MutationUnknown)
}
