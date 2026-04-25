package handler

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"soroban-studio-backend/internal/model"
)

// InterfaceHandler parses Rust pub fn signatures from contract source files.
type InterfaceHandler struct{}

func NewInterfaceHandler() *InterfaceHandler { return &InterfaceHandler{} }

// Handle processes POST /contract/interface
func (h *InterfaceHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req model.InterfaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Build the path prefix used to scope parsing to a single contract.
	// Empty contract_path means "scan every lib.rs" — the legacy behavior
	// preserved so older clients keep working. Anything else restricts to
	// files inside <contract_path>/src/ to avoid mixing pub fns from sibling
	// crates (which produces invoke commands that target the wrong contract).
	prefix := ""
	if req.ContractPath != "" {
		prefix = strings.TrimSuffix(req.ContractPath, "/") + "/src/"
	}

	var fns []model.ContractFn
	for path, content := range req.Files {
		if !strings.HasSuffix(path, "lib.rs") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(path, prefix) {
			continue
		}
		fns = append(fns, parseContractFns(content)...)
	}
	// Dedupe by function name. A contract should never declare the same pub
	// fn twice, but parsing inline tests/mods can occasionally surface dupes.
	// Keep the first occurrence so React keys stay unique downstream.
	seen := make(map[string]struct{}, len(fns))
	deduped := fns[:0]
	for _, fn := range fns {
		if _, ok := seen[fn.Name]; ok {
			continue
		}
		seen[fn.Name] = struct{}{}
		deduped = append(deduped, fn)
	}
	fns = deduped
	if fns == nil {
		fns = []model.ContractFn{}
	}
	// Ensure params is never null
	for i := range fns {
		if fns[i].Params == nil {
			fns[i].Params = []model.FnParam{}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.InterfaceResponse{Functions: fns})
}

// pubFnRe matches: pub fn name(params) or pub fn name(env: Env, params)
var pubFnRe = regexp.MustCompile(`pub\s+fn\s+(\w+)\s*\(([^)]*)\)`)

// paramRe matches: name: Type
var paramRe = regexp.MustCompile(`(\w+)\s*:\s*([^,)]+)`)

func parseContractFns(src string) []model.ContractFn {
	matches := pubFnRe.FindAllStringSubmatch(src, -1)
	var fns []model.ContractFn
	for _, m := range matches {
		name := m[1]
		rawParams := m[2]

		// Skip test helpers and internal fns
		if strings.HasPrefix(name, "test") || name == "new" {
			continue
		}

		var params []model.FnParam
		for _, pm := range paramRe.FindAllStringSubmatch(rawParams, -1) {
			pName := strings.TrimSpace(pm[1])
			pType := strings.TrimSpace(pm[2])
			// Skip env/self params
			if pName == "env" || pName == "self" || pName == "_env" {
				continue
			}
			params = append(params, model.FnParam{Name: pName, Type: pType})
		}

		fns = append(fns, model.ContractFn{
			Name:     name,
			Params:   params,
			Category: inferCategory(name),
		})
	}
	return fns
}

var writePatterns = []string{"set_", "update_", "increment", "decrement", "add_", "remove_", "delete_", "create_", "init", "transfer", "mint", "burn"}
var readPatterns = []string{"get_", "read_", "view_", "query_", "fetch_", "list_", "count", "balance", "total"}

func inferCategory(name string) string {
	lower := strings.ToLower(name)
	for _, p := range readPatterns {
		if strings.HasPrefix(lower, p) || lower == strings.TrimSuffix(p, "_") {
			return "read"
		}
	}
	for _, p := range writePatterns {
		if strings.HasPrefix(lower, p) || lower == strings.TrimSuffix(p, "_") {
			return "write"
		}
	}
	return "unknown"
}
