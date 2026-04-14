package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"soroban-studio-backend/internal/model"
)

type TemplateHandler struct {
	TemplatesDir string
}

func NewTemplateHandler(templatesDir string) *TemplateHandler {
	return &TemplateHandler{
		TemplatesDir: templatesDir,
	}
}

// HandleGetTemplate returns the full structure and content of a project template.
// Usage: GET /api/templates?name=hello-world
func (h *TemplateHandler) HandleGetTemplate(w http.ResponseWriter, r *http.Request) {
	templateName := r.URL.Query().Get("name")
	if templateName == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "template name is required"})
		return
	}

	// Safety: Prevent directory traversal
	if strings.Contains(templateName, "..") {
		WriteJSON(w, http.StatusForbidden, map[string]string{"error": "illegal template name"})
		return
	}

	templatePath := filepath.Join(h.TemplatesDir, templateName)
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "template not found"})
		return
	}

	// We'll build the tree recursively
	// However, the frontend expects: { tree: [rootNode], contents: { path: content } }
	contents := make(map[string]string)
	
	// Use the template name as the root folder name in the explorer
	rootName := templateName

	rootNode, err := h.scanTemplateDir(templatePath, rootName, rootName, contents)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to scan template"})
		return
	}

	response := map[string]interface{}{
		"tree":     []model.FileTreeNode{*rootNode},
		"contents": contents,
	}

	WriteJSON(w, http.StatusOK, response)
}

func (h *TemplateHandler) scanTemplateDir(path, name, idPath string, contents map[string]string) (*model.FileTreeNode, error) {
	node := &model.FileTreeNode{
		Id:   idPath,
		Name: name,
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	// Use folder type for directories
	node.Type = "folder"
	node.Children = []model.FileTreeNode{}

	for _, entry := range entries {
		entryName := entry.Name()

		// Ignore .git related files
		if entryName == ".git" || strings.HasPrefix(entryName, ".git") {
			continue
		}

		entryPath := filepath.Join(path, entryName)
		entryID := idPath + "/" + entryName

		if entry.IsDir() {
			child, err := h.scanTemplateDir(entryPath, entryName, entryID, contents)
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, *child)
		} else {
			// Read file content
			fileContent, err := os.ReadFile(entryPath)
			if err != nil {
				return nil, err
			}

			// Save to contents map
			contents[entryID] = string(fileContent)

			// Add to tree
			node.Children = append(node.Children, model.FileTreeNode{
				Id:   entryID,
				Name: entryName,
				Type: "file",
			})
		}
	}

	return node, nil
}
