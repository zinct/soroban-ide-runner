package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"soroban-studio-backend/internal/model"
)

const defaultWalletName = "stellar-ide-default"

// WalletHandler handles wallet-related endpoints.
type WalletHandler struct {
	containerName string
}

func NewWalletHandler() *WalletHandler {
	name := os.Getenv("RUNNER_CONTAINER")
	if name == "" {
		name = "soroban-runner"
	}
	return &WalletHandler{containerName: name}
}

// runInContainer runs a stellar command inside the runner container with HOME=/root.
func (h *WalletHandler) runInContainer(args ...string) (string, error) {
	dockerArgs := append([]string{"exec", "--env", "HOME=/root", h.containerName}, args...)
	out, err := exec.Command("docker", dockerArgs...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// InitDefault creates and funds the default wallet. Returns the status.
func (h *WalletHandler) InitDefault() (model.WalletStatusResponse, error) {
	output, err := h.runInContainer("stellar", "keys", "generate", defaultWalletName, "--network", "testnet", "--fund")
	alreadyExists := strings.Contains(output, "already exists") ||
		strings.Contains(output, "already generated") ||
		strings.Contains(output, "An identity with the name")
	if err != nil && !alreadyExists {
		return model.WalletStatusResponse{}, fmt.Errorf("%s", output)
	}
	return h.fetchStatus(), nil
}

// HandleInit handles POST /wallet/default/init
func (h *WalletHandler) HandleInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	output, err := h.runInContainer("stellar", "keys", "generate", defaultWalletName, "--network", "testnet", "--fund")

	log.Printf("[wallet] init output: %q err: %v", output, err)

	alreadyExists := strings.Contains(output, "already exists") ||
		strings.Contains(output, "already generated") ||
		strings.Contains(output, "An identity with the name")
	if err != nil && !alreadyExists {
		errMsg := output
		if errMsg == "" {
			errMsg = err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
		return
	}

	status := h.fetchStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// HandleRegisterFreighter handles POST /wallet/freighter/register
// It registers a Freighter public key as a named identity in the runner container.
// stellar keys add freighter-wallet --public-key <address>
func (h *WalletHandler) HandleRegisterFreighter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Address == "" {
		http.Error(w, `{"error":"address required"}`, http.StatusBadRequest)
		return
	}
	// Remove existing entry first (ignore error), then add
	h.runInContainer("stellar", "keys", "remove", "freighter-wallet")
	out, err := h.runInContainer("stellar", "keys", "add", "freighter-wallet", "--public-key", req.Address)
	log.Printf("[wallet] register freighter output: %q err: %v", out, err)
	if err != nil && !strings.Contains(out, "already") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": out})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "name": "freighter-wallet"})
}
func (h *WalletHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	status := h.fetchStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (h *WalletHandler) fetchStatus() model.WalletStatusResponse {
	address, err := h.runInContainer("stellar", "keys", "address", defaultWalletName)
	if err != nil || address == "" {
		return model.WalletStatusResponse{Exists: false, Name: defaultWalletName}
	}
	balance, funded := getBalanceXLM(address)
	return model.WalletStatusResponse{
		Exists:  true,
		Funded:  funded,
		Name:    defaultWalletName,
		Address: address,
		Balance: balance,
	}
}

// getBalanceXLM queries Horizon testnet for the native XLM balance.
func getBalanceXLM(address string) (balance string, funded bool) {
	resp, err := http.Get("https://horizon-testnet.stellar.org/accounts/" + address)
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", false
	}
	defer resp.Body.Close()
	var result struct {
		Balances []struct {
			AssetType string `json:"asset_type"`
			Balance   string `json:"balance"`
		} `json:"balances"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", true
	}
	for _, b := range result.Balances {
		if b.AssetType == "native" {
			return b.Balance, true
		}
	}
	return "", true
}
