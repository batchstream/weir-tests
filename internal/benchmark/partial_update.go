package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	weir "github.com/batchstream/weir-go"
)

const partialUpdateMethod = "partial-update-v1: one existing product per single-record call; flat doc overlay changes batch_sequence on every operation, preserves all other source fields; two disjoint keys per sequential worker; no Lua, external read, seller mutation, or retry"

func partialUpdateSources() map[string]string {
	sum := sha256.Sum256([]byte(partialUpdateMethod))
	sources := map[string]string{"partial_update": hex.EncodeToString(sum[:])}
	return sources
}

func partialUpdateMutation(worker, page int) crawlerMutation {
	id := crawlerID(worker, page%2, "product")
	document := map[string]any{
		"batch_sequence": float64(page + 1),
		"title":          fmt.Sprintf("Partial update %d %s", page, id),
		"available":      (page/2)%2 == 0,
		"prices":         []any{float64(20 + page)},
	}
	mutation := crawlerMutation{ID: id, Product: true, Document: document}
	return mutation
}

func partialUpdateMerge(current map[string]any, mutation crawlerMutation) map[string]any {
	result := crawlerClone(current).(map[string]any)
	for field, value := range mutation.Document {
		result[field] = crawlerClone(value)
	}
	return result
}

func partialUpdateRequest(config crawlerClientConfig, mutation crawlerMutation) (*weir.MutateRequest, error) {
	payload := map[string]any{"doc": mutation.Document}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	expression := &weir.Document{ContentType: "application/vnd.weir.search-update.v1+json", Data: raw}
	completion := weir.MutationQueryVisible
	if config.Visibility == "application_acknowledged" {
		completion = weir.MutationApplicationAcknowledged
	}
	visibility := &weir.MutationOptions{Visibility: completion}
	request := &weir.MutateRequest{Resource: crawlerIndex(config.Namespace, true) + "/s:" + mutation.ID, Action: weir.MutationAtomicTransform, BackendExpression: expression, Options: visibility}
	return request, nil
}
