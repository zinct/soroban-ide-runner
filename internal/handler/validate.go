package handler

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"soroban-studio-backend/internal/model"
)

// ValidateHandler handles POST /validate/project
type ValidateHandler struct {
	helloWorldReadme   string
	workshopReadme     string
}

func NewValidateHandler(helloWorldReadme, workshopReadme string) *ValidateHandler {
	return &ValidateHandler{
		helloWorldReadme: helloWorldReadme,
		workshopReadme:   workshopReadme,
	}
}

func (h *ValidateHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req model.ValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	category := req.Category
	if category != "full-stack" {
		category = "ec-level"
	}

	checks := runChecks(req, category, h.helloWorldReadme, h.workshopReadme)
	status := "valid"
	var failedRemarks []string
	for _, c := range checks {
		if c.Required && c.Status == "fail" {
			status = "invalid"
			failedRemarks = append(failedRemarks, "• "+c.Label+": "+c.Message+" "+c.FixHint)
		}
	}

	remarks := ""
	if len(failedRemarks) > 0 {
		remarks = "The following required checks failed:\n" + strings.Join(failedRemarks, "\n")
	} else {
		remarks = "All required checks passed. Great work!"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.ValidateResponse{
		Category: category,
		Status:   status,
		Checks:   checks,
		Remarks:  remarks,
	})
}

// ─── Rule Runners ─────────────────────────────────────────────────────────────

func runChecks(req model.ValidateRequest, category, hwReadme, wsReadme string) []model.CheckResult {
	var checks []model.CheckResult

	// Common checks
	checks = append(checks, checkRepoName(req.RepoName))
	checks = append(checks, checkContractFolderRenamed(req.Files))

	// README checks
	readme := findReadme(req.Files)
	checks = append(checks, checkReadmeNotTemplate(readme, hwReadme, wsReadme))
	checks = append(checks, checkReadmeTitle(readme))
	checks = append(checks, checkReadmeDescription(readme))
	checks = append(checks, checkReadmeVision(readme))
	checks = append(checks, checkReadmeKeyFeatures(readme))
	checks = append(checks, checkReadmeDeployedDetails(readme))
	checks = append(checks, checkReadmeFutureScope(readme))

	// Code integrity (EC Level)
	checks = append(checks, checkLibrsNotHelloWorld(req.Files))
	checks = append(checks, checkLibrsHasProjectLogic(req.Files))

	if category == "full-stack" {
		checks = append(checks, checkReadmeUIScreenshots(readme))
		checks = append(checks, checkReadmeSetupGuide(readme))
		checks = append(checks, checkFullStackHasFrontend(req.Files))
		checks = append(checks, checkFullStackHasContract(req.Files))
		checks = append(checks, checkFullStackHasIntegration(req.Files))
	}

	return checks
}

// ─── Common Checks ────────────────────────────────────────────────────────────

var genericRepoNames = []string{
	"stellar-project", "smartcontract", "soroban-project", "my-project",
	"hello-world", "test", "project", "contract", "blockchain", "dapp",
	"stellar", "soroban", "notes", "hello_world",
}

func checkRepoName(name string) model.CheckResult {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, g := range genericRepoNames {
		if lower == g || lower == strings.ReplaceAll(g, "-", "_") {
			return model.CheckResult{
				ID: "repo-name-quality", Label: "Repository Name Quality", Required: true,
				Status:  "fail",
				Message: "Repository name '" + name + "' is too generic.",
				FixHint: "Rename your repository to something project-specific (e.g. 'stellar-voting-dapp').",
			}
		}
	}
	if len(lower) < 5 {
		return model.CheckResult{
			ID: "repo-name-quality", Label: "Repository Name Quality", Required: true,
			Status:  "fail",
			Message: "Repository name is too short.",
			FixHint: "Use a descriptive name with at least 5 characters.",
		}
	}
	return model.CheckResult{
		ID: "repo-name-quality", Label: "Repository Name Quality", Required: true,
		Status: "pass", Message: "Repository name looks project-specific.",
	}
}

var defaultContractFolders = []string{"hello-world", "hello_world", "notes"}

func checkContractFolderRenamed(files map[string]string) model.CheckResult {
	for path := range files {
		parts := strings.Split(path, "/")
		// contracts/<folder>/...
		if len(parts) >= 2 && parts[0] == "contracts" {
			folder := strings.ToLower(parts[1])
			for _, d := range defaultContractFolders {
				if folder == d {
					return model.CheckResult{
						ID: "contracts-folder-renamed", Label: "Contract Folder Renamed", Required: true,
						Status:  "fail",
						Message: "Contract folder '" + parts[1] + "' is a default template name.",
						FixHint: "Rename contracts/" + parts[1] + " to a project-specific name.",
					}
				}
			}
		}
	}
	return model.CheckResult{
		ID: "contracts-folder-renamed", Label: "Contract Folder Renamed", Required: true,
		Status: "pass", Message: "Contract folder has a project-specific name.",
	}
}

// ─── README Checks ────────────────────────────────────────────────────────────

func findReadme(files map[string]string) string {
	for path, content := range files {
		base := strings.ToLower(path)
		if base == "readme.md" || strings.HasSuffix(base, "/readme.md") {
			return content
		}
	}
	return ""
}

func similarity(a, b string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	aWords := wordSet(a)
	bWords := wordSet(b)
	common := 0
	for w := range aWords {
		if bWords[w] {
			common++
		}
	}
	total := len(aWords) + len(bWords) - common
	if total == 0 {
		return 1
	}
	return float64(common) / float64(total)
}

func wordSet(s string) map[string]bool {
	words := regexp.MustCompile(`\w+`).FindAllString(strings.ToLower(s), -1)
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

func checkReadmeNotTemplate(readme, hwReadme, wsReadme string) model.CheckResult {
	cr := model.CheckResult{
		ID: "readme-not-template-copy", Label: "README Not Template Copy", Required: true,
	}
	if readme == "" {
		cr.Status = "fail"
		cr.Message = "README.md is missing."
		cr.FixHint = "Add a README.md with project-specific content."
		return cr
	}
	if len(readme) < 300 {
		cr.Status = "fail"
		cr.Message = "README.md is too short to be meaningful."
		cr.FixHint = "Expand your README with project description, features, and deployment details."
		return cr
	}
	simHW := similarity(readme, hwReadme)
	simWS := similarity(readme, wsReadme)
	if simHW > 0.75 || simWS > 0.75 {
		cr.Status = "fail"
		cr.Message = "README.md appears to be a near-copy of the template."
		cr.FixHint = "Customize your README with your project's unique content."
		return cr
	}
	cr.Status = "pass"
	cr.Message = "README.md appears to be meaningfully customized."
	return cr
}

func hasHeading(readme, keyword string) bool {
	lower := strings.ToLower(readme)
	kw := strings.ToLower(keyword)
	// Match ## heading containing keyword
	lines := strings.Split(lower, "\n")
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") && strings.Contains(line, kw) {
			return true
		}
	}
	return false
}

func checkReadmeTitle(readme string) model.CheckResult {
	cr := model.CheckResult{ID: "readme-project-title", Label: "README: Project Title", Required: true}
	if readme == "" {
		cr.Status = "fail"; cr.Message = "README missing."; return cr
	}
	// First # heading
	for _, line := range strings.Split(readme, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && len(trimmed) > 3 {
			cr.Status = "pass"; cr.Message = "Project title found: " + trimmed[2:]
			return cr
		}
	}
	cr.Status = "fail"
	cr.Message = "No top-level project title (# Title) found."
	cr.FixHint = "Add a # Project Title as the first heading in README.md."
	return cr
}

func checkReadmeDescription(readme string) model.CheckResult {
	cr := model.CheckResult{ID: "readme-project-description", Label: "README: Project Description", Required: true}
	if hasHeading(readme, "description") || hasHeading(readme, "about") || hasHeading(readme, "overview") {
		cr.Status = "pass"; cr.Message = "Project description section found."
	} else {
		cr.Status = "fail"
		cr.Message = "No project description section found."
		cr.FixHint = "Add a ## Project Description section to README.md."
	}
	return cr
}

func checkReadmeVision(readme string) model.CheckResult {
	cr := model.CheckResult{ID: "readme-project-vision", Label: "README: Project Vision", Required: true}
	if hasHeading(readme, "vision") {
		cr.Status = "pass"; cr.Message = "Project vision section found."
	} else {
		cr.Status = "fail"
		cr.Message = "No project vision section found."
		cr.FixHint = "Add a ## Project Vision section to README.md."
	}
	return cr
}

func checkReadmeKeyFeatures(readme string) model.CheckResult {
	cr := model.CheckResult{ID: "readme-key-features", Label: "README: Key Features", Required: true}
	if hasHeading(readme, "feature") || hasHeading(readme, "features") {
		cr.Status = "pass"; cr.Message = "Key features section found."
	} else {
		cr.Status = "fail"
		cr.Message = "No key features section found."
		cr.FixHint = "Add a ## Key Features section to README.md."
	}
	return cr
}

// Soroban contract ID: 56-char base32 starting with C
var contractIDRe = regexp.MustCompile(`C[A-Z2-7]{55}`)

// Image reference in markdown: ![...](...)
var imageRe = regexp.MustCompile(`!\[.*?\]\(.*?\)`)

func checkReadmeDeployedDetails(readme string) model.CheckResult {
	cr := model.CheckResult{ID: "readme-deployed-contract-details", Label: "README: Deployed Contract Details", Required: true}
	hasSection := hasHeading(readme, "contract") || hasHeading(readme, "deployed") || hasHeading(readme, "deployment")
	hasContractID := contractIDRe.MatchString(readme)
	hasImage := imageRe.MatchString(readme)

	if !hasSection {
		cr.Status = "fail"
		cr.Message = "No deployed contract details section found."
		cr.FixHint = "Add a ## Contract Details section with the deployed contract ID and a block explorer screenshot."
		return cr
	}
	if !hasContractID {
		cr.Status = "fail"
		cr.Message = "No Soroban contract ID found in README."
		cr.FixHint = "Include the deployed contract address (56-char Stellar contract ID starting with C)."
		return cr
	}
	if !hasImage {
		cr.Status = "fail"
		cr.Message = "No screenshot/image found in README."
		cr.FixHint = "Add a block explorer screenshot as ![screenshot](path/to/image.png)."
		return cr
	}
	cr.Status = "pass"
	cr.Message = "Deployed contract details with contract ID and screenshot found."
	cr.Evidence = contractIDRe.FindString(readme)
	return cr
}

func checkReadmeFutureScope(readme string) model.CheckResult {
	cr := model.CheckResult{ID: "readme-future-scope", Label: "README: Future Scope", Required: true}
	if hasHeading(readme, "future") || hasHeading(readme, "roadmap") || hasHeading(readme, "scope") {
		cr.Status = "pass"; cr.Message = "Future scope section found."
	} else {
		cr.Status = "fail"
		cr.Message = "No future scope/roadmap section found."
		cr.FixHint = "Add a ## Future Scope section describing planned improvements."
	}
	return cr
}

// ─── Code Integrity ───────────────────────────────────────────────────────────

var helloWorldSignatures = []string{
	`fn hello(`, `vec!["Hello", to`, `Symbol::new`, `pub fn hello`,
}

func checkLibrsNotHelloWorld(files map[string]string) model.CheckResult {
	cr := model.CheckResult{ID: "librs-not-hello-world", Label: "Contract: Not Hello World Template", Required: true}
	for path, content := range files {
		if strings.HasSuffix(path, "lib.rs") {
			lower := strings.ToLower(content)
			matchCount := 0
			for _, sig := range helloWorldSignatures {
				if strings.Contains(lower, strings.ToLower(sig)) {
					matchCount++
				}
			}
			if matchCount >= 2 {
				cr.Status = "fail"
				cr.Message = "Contract lib.rs appears to be the hello-world template."
				cr.FixHint = "Replace the hello-world contract logic with your project-specific implementation."
				return cr
			}
		}
	}
	cr.Status = "pass"
	cr.Message = "Contract does not appear to be the hello-world template."
	return cr
}

func checkLibrsHasProjectLogic(files map[string]string) model.CheckResult {
	cr := model.CheckResult{ID: "librs-project-logic-present", Label: "Contract: Project Logic Present", Required: true}
	for path, content := range files {
		if strings.HasSuffix(path, "lib.rs") {
			// Must have at least one pub fn beyond hello/increment
			fns := parseContractFns(content)
			projectFns := 0
			for _, fn := range fns {
				if fn.Name != "hello" && fn.Name != "increment" && fn.Name != "get_count" {
					projectFns++
				}
			}
			if projectFns == 0 {
				cr.Status = "fail"
				cr.Message = "Contract only has placeholder functions."
				cr.FixHint = "Implement project-specific contract functions."
				return cr
			}
			cr.Status = "pass"
			cr.Message = "Contract has project-specific logic."
			return cr
		}
	}
	cr.Status = "fail"
	cr.Message = "No lib.rs found in contracts."
	cr.FixHint = "Ensure your contract source is in contracts/<name>/src/lib.rs."
	return cr
}

// ─── Full Stack Checks ────────────────────────────────────────────────────────

func checkReadmeUIScreenshots(readme string) model.CheckResult {
	cr := model.CheckResult{ID: "readme-ui-screenshots", Label: "README: UI Screenshots", Required: true}
	hasSection := hasHeading(readme, "screenshot") || hasHeading(readme, "ui") || hasHeading(readme, "demo")
	hasImg := imageRe.MatchString(readme)
	if hasSection && hasImg {
		cr.Status = "pass"; cr.Message = "UI screenshots section with image found."
	} else {
		cr.Status = "fail"
		cr.Message = "No UI screenshots section with images found."
		cr.FixHint = "Add a ## Screenshots section with app UI images."
	}
	return cr
}

func checkReadmeSetupGuide(readme string) model.CheckResult {
	cr := model.CheckResult{ID: "readme-project-setup-guide", Label: "README: Setup Guide", Required: true}
	if hasHeading(readme, "setup") || hasHeading(readme, "install") || hasHeading(readme, "getting started") || hasHeading(readme, "run") {
		cr.Status = "pass"; cr.Message = "Setup/installation guide found."
	} else {
		cr.Status = "fail"
		cr.Message = "No setup guide found."
		cr.FixHint = "Add a ## Getting Started or ## Setup section with installation steps."
	}
	return cr
}

func checkFullStackHasFrontend(files map[string]string) model.CheckResult {
	cr := model.CheckResult{ID: "fullstack-has-frontend", Label: "Full Stack: Frontend Present", Required: true}
	for path := range files {
		lower := strings.ToLower(path)
		if strings.Contains(lower, "package.json") || strings.Contains(lower, "index.html") ||
			strings.Contains(lower, "src/app") || strings.Contains(lower, "src/pages") ||
			strings.Contains(lower, "src/components") {
			cr.Status = "pass"; cr.Message = "Frontend files detected."
			return cr
		}
	}
	cr.Status = "fail"
	cr.Message = "No frontend files detected."
	cr.FixHint = "Add a frontend app (React/Next.js/etc.) with package.json."
	return cr
}

func checkFullStackHasContract(files map[string]string) model.CheckResult {
	cr := model.CheckResult{ID: "fullstack-has-contract", Label: "Full Stack: Contract Present", Required: true}
	for path := range files {
		if strings.HasSuffix(path, "lib.rs") && strings.Contains(path, "contracts/") {
			cr.Status = "pass"; cr.Message = "Soroban contract detected."
			return cr
		}
	}
	cr.Status = "fail"
	cr.Message = "No Soroban contract found."
	cr.FixHint = "Add a Soroban contract in contracts/<name>/src/lib.rs."
	return cr
}

var integrationPatterns = []string{
	"stellar-sdk", "soroban-client", "contract.call", "contractId",
	"StellarSdk", "@stellar/stellar-sdk", "rpc.getTransaction",
}

func checkFullStackHasIntegration(files map[string]string) model.CheckResult {
	cr := model.CheckResult{ID: "fullstack-has-integration-logic", Label: "Full Stack: Integration Logic", Required: true}
	for _, content := range files {
		for _, pattern := range integrationPatterns {
			if strings.Contains(content, pattern) {
				cr.Status = "pass"; cr.Message = "Stellar SDK integration detected."
				return cr
			}
		}
	}
	cr.Status = "fail"
	cr.Message = "No Stellar SDK integration found in frontend."
	cr.FixHint = "Add Stellar SDK calls to connect your frontend to the deployed contract."
	return cr
}
