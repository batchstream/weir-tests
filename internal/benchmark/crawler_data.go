package benchmark

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// The Lua rules are a frozen translation of PSE 283940ef442b479440e45b39c86988351b92d7b5.
// Only the reconcile early return changes: reconcile and merge execute in one mutation.
//
//go:embed crawlerdata/common.lua
var crawlerCommon string

//go:embed crawlerdata/product.lua
var crawlerProduct string

//go:embed crawlerdata/offer.lua
var crawlerOffer string

//go:embed crawlerdata/native.painless
var crawlerNative string

const crawlerObserved = "2026-10-15T00:00:00Z"

type crawlerMutation struct {
	ReconcileOnly bool           `json:"reconcile_only"`
	ID            string         `json:"id"`
	Product       bool           `json:"product"`
	Base          bool           `json:"base"`
	Document      map[string]any `json:"document"`
	ObservedAt    string         `json:"observed_at"`
	Reconcile     bool           `json:"reconcile_offers"`
}

func crawlerIndex(namespace string, product bool) string {
	if product {
		return namespace + "-products"
	}
	return namespace + "-offers"
}

func crawlerID(worker, record int, suffix string) string {
	return fmt.Sprintf("worker-%04d-record-%04d-%s", worker, record, suffix)
}

func crawlerDocument(id string, product bool) map[string]any {
	doc := map[string]any{"uid": id, "id": id, "platform": "fixture-market", "country": "US", "url": "https://market.example/" + id, "uids": []any{id}, "hostnames": []any{"market.example"}, "first_found_at": "2026-01-01T00:00:00Z", "last_found_at": "2026-01-01T00:00:00Z"}
	if product {
		doc["title"] = "Crawler product " + id
		doc["description"] = strings.Repeat("durable product description ", 80)
		doc["brand"] = "EXAMPLE"
		doc["category"] = []any{"home", "tools"}
		doc["gallery"] = []any{"https://images.example/a.jpg"}
		doc["prices"] = []any{19.5}
		doc["allowed_countries"] = []any{"US"}
		doc["languages"] = []any{"en"}
		doc["available"] = true
		var stocks, solds, comments []any
		for n := range 20 {
			stock := map[string]any{"stock": float64(n + 1), "variables": map[string]any{"color": "red"}}
			sold := map[string]any{"sold": float64(n + 1), "period_hours": float64(24), "record_at": "2026-09-01T00:00:00Z"}
			comment := map[string]any{"description": fmt.Sprintf("review-%d", n), "score": float64(4), "date": "2026-09-01T00:00:00Z"}
			stocks = append(stocks, stock)
			solds = append(solds, sold)
			comments = append(comments, comment)
		}
		doc["stocks"], doc["solds"], doc["comments"] = stocks, solds, comments
		old := map[string]any{"uid": id + "-retired", "id": "retired", "url": "https://market.example/retired", "product_price": float64(17)}
		doc["offers"] = []any{old}
	} else {
		doc["name"] = "Seller " + id
		doc["bio"] = strings.Repeat("seller profile ", 12)
		doc["business_name"] = "Example company"
		doc["sold"] = float64(50)
		doc["has_paypal"] = true
		address := map[string]any{"raw": "1 Example road", "country": "US"}
		contact := map[string]any{"type": "email", "value": "fixture@example.invalid"}
		tracking := map[string]any{"ga4": []any{"G-FIXTURE"}}
		doc["addresses"] = []any{address}
		doc["contacts"] = []any{contact}
		doc["tracking_ids"] = tracking
	}
	return doc
}

func crawlerPage(worker, page, size int) []crawlerMutation {
	mutations := make([]crawlerMutation, 0, size*3)
	variant := (page / 2) % 2
	for offset := range size {
		record := (page%2)*size + offset
		id := crawlerID(worker, record, "product")
		doc := map[string]any{"uid": id, "id": id, "platform": "fixture-market", "country": "US", "url": "https://market.example/" + id, "title": fmt.Sprintf("Crawler product variant %d %s", variant, id), "brand": "example", "description": strings.Repeat("observed product description ", 80), "available": variant == 0, "sold_by_platform": false, "allowed_countries": []any{"US", "CA"}, "languages": []any{"en", "zh"}, "prices": []any{float64(20 + variant)}, "comment_count": float64(100 + variant)}
		stock := map[string]any{"stock": float64(100 + variant), "variables": map[string]any{"color": "blue"}}
		sold := map[string]any{"sold": float64(100 + variant), "period_hours": float64(24), "record_at": "2026-10-15T00:00:00Z"}
		comment := map[string]any{"description": fmt.Sprintf("new-review-%d", variant), "score": float64(5), "date": "2026-10-15T00:00:00Z"}
		doc["stocks"] = []any{stock}
		doc["solds"] = []any{sold}
		doc["comments"] = []any{comment}
		var offers []any
		for seller := range 2 {
			sid := crawlerID(worker, record, fmt.Sprintf("seller-%d", seller))
			offer := map[string]any{"uid": sid, "id": fmt.Sprintf("seller-%d", seller), "url": "https://market.example/" + sid, "product_price": float64(20 + variant)}
			offers = append(offers, offer)
			sellerDoc := map[string]any{"uid": sid, "id": sid, "platform": "fixture-market", "name": fmt.Sprintf("Seller variant %d", variant), "url": "https://market.example/" + sid, "country": "US", "has_paypal": variant == 0, "sold": float64(100 + variant), "uids": []any{sid, "external-" + sid}}
			address := map[string]any{"raw": fmt.Sprintf("%d New road", variant+2), "country": "US"}
			contact := map[string]any{"type": "phone", "value": fmt.Sprintf("555-000%d", variant)}
			tracking := map[string]any{"ga4": []any{"G-FIXTURE", fmt.Sprintf("G-NEW%d", variant)}}
			sellerDoc["addresses"] = []any{address}
			sellerDoc["contacts"] = []any{contact}
			sellerDoc["tracking_ids"] = tracking
			mutation := crawlerMutation{ID: sid, Document: sellerDoc, ObservedAt: crawlerObserved}
			mutations = append(mutations, mutation)
		}
		doc["offers"] = offers
		mutation := crawlerMutation{ID: id, Product: true, Document: doc, ObservedAt: crawlerObserved, Reconcile: true}
		mutations = append(mutations, mutation)
	}
	// A shared existing seller is touched by every hundredth page; all fields remain unchanged.
	if (worker+page)%100 == 0 {
		doc := map[string]any{"uid": "shared-seller", "name": "placeholder"}
		mutation := crawlerMutation{ID: "shared-seller", Base: true, Document: doc, ObservedAt: crawlerObserved}
		mutations = append(mutations, mutation)
	}
	return mutations
}

func crawlerClone(value any) any {
	raw, _ := json.Marshal(value)
	var copy any
	_ = json.Unmarshal(raw, &copy)
	return copy
}

func crawlerUnion(before, after []any) []any {
	result := append([]any{}, before...)
	for _, value := range after {
		found := false
		for _, old := range result {
			if reflect.DeepEqual(old, value) {
				found = true
				break
			}
		}
		if !found {
			result = append(result, crawlerClone(value))
		}
	}
	return result
}

// The independent Go oracle uses domain field groups, not Lua or the native script.
func crawlerMerge(current map[string]any, mutation crawlerMutation) map[string]any {
	if mutation.Base && current != nil {
		return crawlerClone(current).(map[string]any)
	}
	result := crawlerClone(current).(map[string]any)
	incoming := mutation.Document
	for key, value := range incoming {
		if key == "offers" || key == "stocks" || key == "solds" || key == "comments" || key == "addresses" || key == "contacts" || key == "tracking_ids" {
			continue
		}
		switch key {
		case "uids", "allowed_countries", "languages":
			before, _ := result[key].([]any)
			result[key] = crawlerUnion(before, value.([]any))
		case "available", "sold_by_platform", "has_paypal":
			if value == true {
				result[key] = true
			} else {
				delete(result, key)
			}
		case "brand":
			result[key] = strings.ToUpper(value.(string))
		default:
			result[key] = crawlerClone(value)
		}
	}
	if mutation.Product {
		uids, _ := result["uids"].([]any)
		result["uids"] = crawlerUnion(uids, []any{incoming["uid"]})
		oldOffers, _ := result["offers"].([]any)
		if mutation.Reconcile {
			for _, old := range oldOffers {
				offer := old.(map[string]any)
				offer["expiry_detected_at"] = mutation.ObservedAt
				for _, newOffer := range incoming["offers"].([]any) {
					if offer["id"] == newOffer.(map[string]any)["id"] {
						offer["product_price"] = newOffer.(map[string]any)["product_price"]
						offer["expiry_detected_at"] = nil
						break
					}
				}
			}
		}
		var offers []any
		seen := map[string]bool{}
		combined := append(append([]any{}, incoming["offers"].([]any)...), oldOffers...)
		for _, value := range combined {
			uid := value.(map[string]any)["uid"].(string)
			if !seen[uid] {
				offers = append(offers, value)
				seen[uid] = true
			}
		}
		if len(offers) > 50 {
			offers = offers[len(offers)-50:]
		}
		result["offers"] = offers
	}
	for _, field := range []string{"stocks", "solds", "comments", "addresses", "contacts"} {
		values, ok := incoming[field].([]any)
		if !ok {
			continue
		}
		before, _ := result[field].([]any)
		merged := crawlerUnion(before, values)
		limit := 20
		if field == "addresses" {
			limit = 10
		}
		if field != "contacts" && len(merged) > limit {
			merged = merged[len(merged)-limit:]
		}
		result[field] = merged
	}
	if tracking, ok := incoming["tracking_ids"].(map[string]any); ok {
		target, _ := result["tracking_ids"].(map[string]any)
		if target == nil {
			target = map[string]any{}
		}
		for field, values := range tracking {
			before, _ := target[field].([]any)
			target[field] = crawlerUnion(before, values.([]any))
		}
		result["tracking_ids"] = target
	}
	result["last_found_at"] = mutation.ObservedAt
	delete(result, "evicted_at")
	return result
}

func crawlerSources() map[string]string {
	sources := map[string]string{}
	for name, source := range map[string]string{"common": crawlerCommon, "product": crawlerProduct, "offer": crawlerOffer, "native": crawlerNative, "two_step_product": strings.Replace(crawlerProduct, "        reconcile(current, incoming)", "        return reconcile(current, incoming)", 1)} {
		sum := sha256.Sum256([]byte(source))
		sources[name] = hex.EncodeToString(sum[:])
	}
	return sources
}
